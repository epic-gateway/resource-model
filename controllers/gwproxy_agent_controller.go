package controllers

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/network"
)

// GWProxyAgentReconciler reconciles a GWProxy object by performing
// the per-node networking setup needed to enable True Ingress
// tunnels.
type GWProxyAgentReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *GWProxyAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("agent-running-on", os.Getenv("EPIC_NODE_NAME"))
	proxy := &epicv1.GWProxy{}
	epic := &epicv1.EPIC{}
	sg := &epicv1.LBServiceGroup{}
	prefix := &epicv1.ServicePrefix{}
	account := &epicv1.Account{}

	l.Info("reconciling")

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, proxy); err != nil {
		l.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Get the "stack" of CRs to which this proxy belongs:
	// LBServiceGroup, ServicePrefix, Account, and the EPIC
	// singleton. They provide configuration data that we need but that
	// isn't contained in the proxy.
	sgname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: proxy.Labels[epicv1.OwningLBServiceGroupLabel]}
	if err := r.Get(ctx, sgname, sg); err != nil {
		l.Error(err, "Failed to find owning service group", "name", sgname)
		return done, err
	}

	prefixName := types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: sg.Labels[epicv1.OwningServicePrefixLabel]}
	if err := r.Get(ctx, prefixName, prefix); err != nil {
		l.Error(err, "Failed to find owning service prefix", "name", prefixName)
		return done, err
	}

	accountName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: sg.Labels[epicv1.OwningAccountLabel]}
	if err := r.Get(ctx, accountName, account); err != nil {
		return done, err
	}

	epicName := types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: epicv1.ConfigName}
	if err := r.Get(ctx, epicName, epic); err != nil {
		l.Error(err, "Failed to find EPIC config singleton", "name", epicName)
		return done, err
	}

	// Calculate this proxy's public address which we use both when we add
	// and when we delete
	publicAddr := net.ParseIP(proxy.Spec.PublicAddress)
	if publicAddr == nil {
		return done, fmt.Errorf("%s can't be parsed as an IP address", proxy.Spec.PublicAddress)
	}

	// Check if k8s wants to delete this object
	if !proxy.ObjectMeta.DeletionTimestamp.IsZero() {
		// if err := cleanupPFC(log, lb, account.Spec.GroupID); err != nil {
		// 	log.Error(err, "Failed to cleanup PFC")
		// }

		if err := cleanupLinux(ctx, l, r, prefix, proxy.Name, publicAddr, proxy.Spec.PublicPorts); err != nil {
			l.Error(err, "Failed to cleanup Linux")
		}

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		l.Info("removing finalizer")
		if err := RemoveFinalizer(ctx, r.Client, proxy, proxy.AgentFinalizerName(os.Getenv("EPIC_NODE_NAME"))); err != nil {
			return done, err
		}

		return done, nil
	}

	// The proxy is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	l.Info("adding finalizer")
	if err := AddFinalizer(ctx, r.Client, proxy, proxy.AgentFinalizerName(os.Getenv("EPIC_NODE_NAME"))); err != nil {
		return done, err
	}

	// List the endpoints that belong to this proxy.
	reps, err := proxy.ActiveProxyEndpoints(ctx, r.Client)
	if err != nil {
		return done, err
	}

	hasProxy := false

	myAddr, err := epic.Spec.NodeBase.IngressIPAddr()
	if err != nil {
		return done, err
	}

	// Set up each proxy pod belonging to this proxy
	for proxyName, proxyInfo := range proxy.Spec.ProxyInterfaces {

		pl := l.WithValues("proxy", proxyName)

		// Skip if the relevant Envoy isn't running on this host. We can
		// tell because there's an entry in the ProxyInterfaces map but
		// it's not for this host.
		if proxyInfo.EPICNodeAddress != myAddr {
			pl.Info("Not me: status has no proxy interface info for this host", "nodeAddr", proxyInfo.EPICNodeAddress, "myAddr", myAddr)
			continue
		}

		// Skip if the relevant Envoy needs to have its interface data set
		// by the Python daemon.
		if proxyInfo.Name == "" {
			pl.Info("proxy interface info incomplete", "name", proxyName)
			continue
		}

		hasProxy = true
		pl.Info("proxy status proxy veth info", "proxy-name", proxyName, "ifindex", proxyInfo.Index, "ifname", proxyInfo.Name)

		// Set up a service gateway to each client-cluster endpoint on
		// this client-cluster node
		for _, epInfo := range reps {
			pl.Info("setting up rep", "ip", epInfo.Spec)

			// For each client-cluster node that hosts an endpoint we need to
			// set up a tunnel
			clientTunnelMap, hasClientMap := proxy.Spec.GUETunnelMaps[epInfo.Spec.NodeAddress]
			if !hasClientMap {
				pl.Error(fmt.Errorf("no client-side tunnel info for node %s", epInfo.Spec.NodeAddress), "setting up tunnels")
			}

			tunnelInfo, hasTunnelInfo := clientTunnelMap.EPICEndpoints[proxyInfo.EPICNodeAddress]
			if !hasTunnelInfo {
				continue
			}

			// Configure the tunnel, which connects an EPIC node to a
			// client-cluster node
			if err := configureTunnel(l, tunnelInfo); err != nil {
				pl.Error(err, "setting up tunnel", "epic-ip", os.Getenv("EPIC_HOST"), "client-info", tunnelInfo)
			}

			// Set up a service gateway from this proxy to this rep
			pl.Info("setting up rep", "rep", epInfo)
			if err := configureService(l, epInfo.Spec, proxyInfo.Index, tunnelInfo.TunnelID, proxy.Spec.TunnelKey); err != nil {
				pl.Error(err, "setting up service gateway")
			}
		}
	}

	// We don't want to advertise routes unless this host is running an
	// Envoy proxy. We might have been running one before and set up the
	// routes, but if we're not running one now then we need to clean
	// up.
	if hasProxy {
		// Open host ports by updating IPSET tables
		if err := network.AddIpsetEntry(proxy.Spec.PublicAddress, proxy.Spec.PublicPorts); err != nil {
			l.Error(err, "adding ipset entry")
			return done, err
		}
	} else {
		l.Info("noProxy")
		// if err := cleanupLinux(ctx, log, r, prefix, lb, publicAddr); err != nil {
		// 	log.Error(err, "Failed to cleanup Linux")
		// }
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *GWProxyAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWProxy{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *GWProxyAgentReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

func cleanupLinux(ctx context.Context, l logr.Logger, r client.Reader, prefix *epicv1.ServicePrefix, lbName string, publicAddr net.IP, ports []v1.ServicePort) error {
	l.Info("cleanupLinux")

	// remove route
	if err := prefix.RemoveMultusRoute(ctx, r, l, lbName, publicAddr); err != nil {
		l.Error(err, "Failed to delete bridge route")
	}

	// remove IPSet entry
	if err := network.DelIpsetEntry(publicAddr.String(), ports); err != nil {
		l.Error(err, "Failed to delete ipset entry")
	}

	return nil
}
