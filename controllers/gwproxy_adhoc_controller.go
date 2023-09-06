package controllers

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "epic-gateway.org/resource-model/api/v1"
	epicexec "epic-gateway.org/resource-model/internal/exec"
)

// GWProxyAdhocReconciler reconciles a GWProxy object by performing
// the per-node networking setup needed to enable True Ingress
// tunnels.
type GWProxyAdhocReconciler struct {
	client.Client
	RuntimeScheme *runtime.Scheme
	NodeAddress   string
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *GWProxyAdhocReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("node address", r.NodeAddress)
	proxy := &epicv1.GWProxy{}

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, proxy); err != nil {
		l.Info("Can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	l.Info("Reconciling")

	// Set up each proxy pod belonging to this proxy
	endpointMap, hasMap := proxy.Spec.GUETunnelMaps[r.NodeAddress]
	if hasMap {
		for _, tunnelInfo := range endpointMap.EPICEndpoints {
			l.Info("Setting up tunnel", "tunnel-id", tunnelInfo.TunnelID, "epicIP", tunnelInfo.Address, "port", tunnelInfo.Port.Port)

			// Configure the tunnel between the EPIC node and this one.
			if err := setTunnel(l, tunnelInfo.TunnelID, tunnelInfo.Address, r.NodeAddress, tunnelInfo.Port.Port); err != nil {
				l.Error(err, "Setting up tunnel", "epic-ip", os.Getenv("EPIC_HOST"), "client-info", tunnelInfo)
			}

			// Set up a service gateway from this proxy to this rep.
			if err := setService(l, proxy.Spec.TunnelKey, tunnelInfo.TunnelID); err != nil {
				l.Error(err, "Setting up service gateway")
			}

		}
	} else {
		l.Info("No tunnels ending at this node")
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *GWProxyAdhocReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWProxy{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *GWProxyAdhocReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// setTunnel sets the parameters needed by one PFC tunnel.
func setTunnel(log logr.Logger, tunnelID uint32, tunnelAddr string, myAddr string, tunnelPort int32) error {
	return epicexec.RunScript(log, fmt.Sprintf("/opt/acnodal/bin/cli_tunnel set %[1]d %[3]s %[4]d %[2]s %[4]d", tunnelID, tunnelAddr, myAddr, tunnelPort))
}

// setService sets the parameters needed by one PFC service.
func setService(log logr.Logger, tunnelAuth string, tunnelID uint32) error {
	return epicexec.RunScript(log, fmt.Sprintf("/opt/acnodal/bin/cli_service set-node %[1]d %[2]d %[3]s %[4]d", tunnelID>>16, tunnelID&0xff, tunnelAuth, tunnelID))
}
