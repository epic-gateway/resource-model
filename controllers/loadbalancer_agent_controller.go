package controllers

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	epicexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
	"gitlab.com/acnodal/epic/resource-model/internal/network"
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

	// If any of this LB's Envoy proxy pods need to have their interface
	// data set by the Python daemon then back off and retry later
	if len(lb.Spec.ProxyInterfaces) < 1 {
		log.Info("proxy interface info missing")
		return tryAgain, nil
	}
	for proxyName, proxyInfo := range lb.Spec.ProxyInterfaces {
		if proxyInfo.Name == "" {
			log.Info("proxy interface info incomplete", "name", proxyName)
			return tryAgain, nil
		}
	}

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
	publicAddr := net.ParseIP(lb.Spec.PublicAddress)
	if publicAddr == nil {
		return done, fmt.Errorf("%s can't be parsed as an IP address", lb.Spec.PublicAddress)
	}

	// List the endpoints that belong to this LB.
	reps, err := listActiveLBEndpoints(ctx, r, lb)
	if err != nil {
		return done, err
	}

	// Attempt to set up each proxy pod belonging to this LB
	for proxyName, proxyInfo := range lb.Spec.ProxyInterfaces {

		log = r.Log.WithValues("proxy", proxyName)

		// Skip if the relevant Envoy isn't running on this host. We can
		// tell because there's an entry in the ProxyInterfaces map but
		// it's not for this host.
		if proxyInfo.EPICNodeAddress != os.Getenv("EPIC_HOST") {
			log.Info("Not me: status has no proxy interface info for this host")
			continue
		}
		log.Info("LB status proxy veth info", "proxy-name", proxyName, "ifindex", proxyInfo.Index, "ifname", proxyInfo.Name)

		// 	// Check if k8s wants to delete this object
		// 	if !lb.ObjectMeta.DeletionTimestamp.IsZero() {
		// 		// Remove network and PFC configuration
		// 		if err := r.cleanup(log, lb, account.Spec.GroupID, prefix.Spec.MultusBridge); err != nil {
		// 			log.Error(err, "Failed to cleanup PFC")
		// 		}

		// 		// remove route
		// 		if err := prefix.RemoveMultusRoute(ctx, r, log, lb.Name, publicAddr); err != nil {
		// 			log.Error(err, "Failed to delete bridge route")
		// 		}

		// 	} else {

		// Setup host routing
		log.Info("adding route", "prefix", prefix.Name, "address", lb.Spec.PublicAddress)
		if err := prefix.AddMultusRoute(publicAddr); err != nil {
			return done, err
		}

		// Set up the network and PFC
		if err := r.setup(log, lb, proxyInfo); err != nil {
			return done, err
		}

		// For each client-cluster node that hosts an endpoint we need to
		// set up a tunnel
		for _, clNodeInfo := range lb.Spec.GUETunnelMaps {
			tunnelInfo, hasTunnelInfo := clNodeInfo.EPICEndpoints[os.Getenv("EPIC_HOST")]
			if hasTunnelInfo {

				// Configure the tunnel, which connects an EPIC node to a
				// client-cluster node
				if err := configureTunnel(log, tunnelInfo); err != nil {
					log.Error(err, "setting up tunnel", "epic-ip", os.Getenv("EPIC_HOST"), "client-info", tunnelInfo)
				}

				// Set up a service gateway to each client-cluster endpoint on
				// this client-cluster node
				for _, epInfo := range reps {
					if err := configureService(log, epInfo.Spec, proxyInfo.Index, account.Spec.GroupID, lb.Spec.ServiceID, tunnelInfo.TunnelID, lb.Spec.TunnelKey); err != nil {
						log.Error(err, "setting up service gateway")
					}
				}
			}
		}
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

func configureTunnel(l logr.Logger, ep epicv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel get %[1]d || /opt/acnodal/bin/cli_tunnel set %[1]d %[2]s %[3]d 0 0", ep.TunnelID, ep.Address, ep.Port.Port)
	return epicexec.RunScript(l, script)
}

func configureService(l logr.Logger, ep epicv1.RemoteEndpointSpec, ifindex int, groupID uint16, serviceID uint16, tunnelID uint32, tunnelAuth string) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service set-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	return epicexec.RunScript(l, script)
}

// cleanup undoes the PFC setup that we did for this RemoteEndpoint.
func cleanup(l logr.Logger, ep epicv1.RemoteEndpointSpec, ifindex int, tunnelID uint32, groupID uint16, serviceID uint16) error {
	script1 := fmt.Sprintf("/opt/acnodal/bin/cli_service del-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, "unused", tunnelID, ep.Address, ep.Port.Port, ifindex)
	script2 := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", tunnelID)
	err1 := epicexec.RunScript(l, script1)
	err2 := epicexec.RunScript(l, script2)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

func (r *LoadBalancerAgentReconciler) deleteTunnel(l logr.Logger, ep epicv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", ep.TunnelID)
	return epicexec.RunScript(l, script)
}

func (r *LoadBalancerAgentReconciler) deleteService(l logr.Logger, groupID uint16, serviceID uint16) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service del %[1]d %[2]d", groupID, serviceID)
	return epicexec.RunScript(l, script)
}

// setup sets up the networky stuff that we need to do for lb.
func (r *LoadBalancerAgentReconciler) setup(l logr.Logger, lb *epicv1.LoadBalancer, ifInfo epicv1.ProxyInterfaceInfo) error {

	// Open host ports by updating IPSET tables
	if err := network.AddIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts); err != nil {
		l.Error(err, "adding ipset entry")
		return err
	}

	return nil
}

// cleanup undoes the setup that we did for this lb.
func (r *LoadBalancerAgentReconciler) cleanup(l logr.Logger, lb *epicv1.LoadBalancer, groupID uint16, bridgeName string) error {
	// remove IPSet entry
	network.DelIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts)

	// remove the endpoint PFC "services"
	r.deleteService(l, groupID, lb.Spec.ServiceID)

	// remove the PFC tunnels
	for _, epicEPMap := range lb.Spec.GUETunnelMaps {
		for _, tunnel := range epicEPMap.EPICEndpoints {
			r.deleteTunnel(l, tunnel)
		}
	}

	return nil
}
