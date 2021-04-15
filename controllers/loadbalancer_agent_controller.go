package controllers

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	epicexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
	"gitlab.com/acnodal/epic/resource-model/internal/network"
	"gitlab.com/acnodal/epic/resource-model/internal/pfc"
)

// LoadBalancerAgentReconciler reconciles a LoadBalancer object by
// performing the per-node networking setup needed to enable True
// Ingress tunnels.
type LoadBalancerAgentReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *LoadBalancerAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("loadbalancer", req.NamespacedName)
	lb := &epicv1.LoadBalancer{}
	sg := &epicv1.LBServiceGroup{}
	prefix := &epicv1.ServicePrefix{}
	account := &epicv1.Account{}

	log.Info("reconciling")

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, lb); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// We need this LB's veth info so if the python daemon hasn't filled
	// it in yet then we'll back off and give it more time
	if lb.Status.ProxyInterfaces == nil || len(lb.Status.ProxyInterfaces) == 0 {
		log.Info("status has no proxy interfaces info")
		return tryAgain, nil
	}

	// Return if the relevant Envoy isn't running on this host. We can
	// tell because there will be an entry in the ProxyInterfaces map
	// but it's not for this host.
	proxyInfo, haveProxyInfo := lb.Status.ProxyInterfaces[os.Getenv("EPIC_HOST")]
	if !haveProxyInfo {
		log.Info("Not me: status has no proxy interface info for this host")
		return done, nil
	}
	log.Info("LB status veth info", "ifindex", proxyInfo.Index, "ifname", proxyInfo.Name)

	// Get the "stack" of CRs to which this LB belongs: LBServiceGroup,
	// ServicePrefix, and Account. They provide configuration data that
	// we need but that isn't contained in the LB.
	sgname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Labels[epicv1.OwningLBServiceGroupLabel]}
	if err := r.Get(ctx, sgname, sg); err != nil {
		log.Error(err, "Failed to find owning service group", "name", sgname)
		return done, err
	}

	prefixName := types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: sg.Labels[epicv1.OwningServicePrefixLabel]}
	if err := r.Get(ctx, prefixName, prefix); err != nil {
		log.Error(err, "Failed to find owning service prefix", "name", prefixName)
		return done, err
	}

	accountName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: sg.Labels[epicv1.OwningAccountLabel]}
	if err := r.Get(ctx, accountName, account); err != nil {
		return done, err
	}

	// Calculate this LB's public address which we use both when we add
	// and when we delete
	subnet, err := prefix.Spec.SubnetIPNet()
	if err != nil {
		return done, err
	}
	_, publicAddr, err := net.ParseCIDR(lb.Spec.PublicAddress + "/32")
	if err != nil {
		return done, err
	}
	if prefix.Spec.Aggregation == "default" {
		publicAddr.Mask = subnet.Mask
	} else {
		_, publicAddr, err = net.ParseCIDR(lb.Spec.PublicAddress + prefix.Spec.Aggregation)
		if err != nil {
			return done, err
		}
	}

	// Check if k8s wants to delete this object
	if !lb.ObjectMeta.DeletionTimestamp.IsZero() {
		// Remove network and PFC configuration
		if err := r.cleanup(log, publicAddr, lb, account.Spec.GroupID, prefix.Spec.MultusBridge); err != nil {
			log.Error(err, "Failed to cleanup PFC")
		}

		return done, nil // stop reconciliation as the item is being deleted
	}

	// Set up the network and PFC

	if err = r.setup(log, lb, proxyInfo); err != nil {
		return done, err
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *LoadBalancerAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.LoadBalancer{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *LoadBalancerAgentReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// deleteAddr deletes lbIP from whichever interface has it.
func delAddr(lbIP net.IP) error {
	hostints, _ := net.Interfaces()
	for _, hostint := range hostints {
		addrs, _ := hostint.Addrs()
		for _, ipnet := range addrs {
			ipaddr, _, _ := net.ParseCIDR(ipnet.String())

			if lbIP.Equal(ipaddr) {
				ifindex, _ := netlink.LinkByIndex(hostint.Index)
				deladdr, _ := netlink.ParseAddr(ipnet.String())
				if err := netlink.AddrDel(ifindex, deladdr); err != nil {
					return fmt.Errorf("could not remove %v from %v: %w", deladdr, ifindex, err)
				}
				return nil
			}
		}
	}

	return fmt.Errorf("could not remove %v: not found on any interface", lbIP)
}

func (r *LoadBalancerAgentReconciler) deleteTunnel(l logr.Logger, ep epicv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", ep.TunnelID)
	return epicexec.RunScript(l, script)
}

func (r *LoadBalancerAgentReconciler) deleteService(l logr.Logger, groupID uint16, serviceID uint16) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service del %[1]d %[2]d", groupID, serviceID)
	return epicexec.RunScript(l, script)
}

func (r *LoadBalancerAgentReconciler) configureTagging(l logr.Logger, ifname string) error {
	err := pfc.AddQueueDiscipline(ifname)
	if err != nil {
		return err
	}
	return pfc.AddFilter(ifname, "ingress", "tag_rx")
}

// setup sets up the networky stuff that we need to do for lb.
func (r *LoadBalancerAgentReconciler) setup(l logr.Logger, lb *epicv1.LoadBalancer, ifInfo epicv1.ProxyInterfaceInfo) error {

	// add the packet tagger to the Envoy pod veth
	if err := r.configureTagging(l, ifInfo.Name); err != nil {
		l.Error(err, "adding packet tagger", "if", ifInfo.Name)
		return err
	}

	// Open host ports by updating IPSET tables
	if err := network.AddIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts); err != nil {
		l.Error(err, "adding ipset entry")
		return err
	}

	return nil
}

// cleanup undoes the setup that we did for this lb.
func (r *LoadBalancerAgentReconciler) cleanup(l logr.Logger, publicAddr *net.IPNet, lb *epicv1.LoadBalancer, groupID uint16, bridgeName string) error {
	// remove IPSet entry
	network.DelIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts)

	// remove route
	if err := network.DelRt(publicAddr); err != nil {
		l.Error(err, "Failed to delete bridge route")
	}

	// remove address
	if err := delAddr(publicAddr.IP); err != nil {
		l.Error(err, "Failed to remove address from bridge")
	}

	// remove the endpoint PFC "services"
	r.deleteService(l, groupID, lb.Spec.ServiceID)

	// remove the PFC tunnels
	for _, tunnel := range lb.Status.GUETunnelEndpoints {
		r.deleteTunnel(l, tunnel)
	}

	return nil
}
