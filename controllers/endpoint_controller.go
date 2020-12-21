package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

// EndpointReconciler reconciles Endpoint objects.
type EndpointReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=endpoints/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *EndpointReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	done := ctrl.Result{Requeue: false}
	tryAgain := ctrl.Result{RequeueAfter: 10 * time.Second}
	ctx := context.TODO()
	log := r.Log.WithValues("endpoint", req.NamespacedName)

	// Get the object that caused the event
	ep := &egwv1.Endpoint{}
	if err := r.Get(ctx, req.NamespacedName, ep); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Check if k8s wants to delete this Endpoint
	if !ep.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if containsString(ep.ObjectMeta.Finalizers, egwv1.EndpointFinalizerName) {
			log.Info("object to be deleted")

			if err := r.cleanupService(log, ep.Spec, ep.Status.ProxyIfindex, ep.Status.TunnelID, ep.Status.GUEKey); err != nil {
				log.Error(err, "Failed to cleanup PFC")
			}

			// remove our finalizer from the list and update the object
			ep.ObjectMeta.Finalizers = removeString(ep.ObjectMeta.Finalizers, egwv1.EndpointFinalizerName)
			if err := r.Update(ctx, ep); err != nil {
				return done, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return done, nil
	}

	// Get the LoadBalancer that owns this Endpoint
	lb := &egwv1.LoadBalancer{}
	lbname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: ep.Spec.LoadBalancer}
	if err := r.Get(ctx, lbname, lb); err != nil {
		log.Error(err, "Failed to find owning load balancer", "name", lbname)
		return done, err
	}

	// If the LB doesn't have its ifindex set then we can't configure so
	// try again in a few seconds
	if lb.Status.ProxyIfindex == 0 {
		log.Info("LB ifindex not set yet")
		return tryAgain, nil
	}

	// Add GUE ingress address/port to the endpoint
	gueEp, err := r.setGUEIngressAddress(ctx, lb, &ep.Spec)
	if err != nil {
		log.Error(err, "Patching LB status")
		return done, err
	}

	// configure the GUE tunnel
	err = r.configureTunnel(log, gueEp)
	if err != nil {
		log.Error(err, "configuring GUE tunnel")
		return done, err
	}

	// configure the GUE "service"
	err = r.configureService(log, ep.Spec, lb.Status.ProxyIfindex, gueEp.TunnelID, lb.Spec.GUEKey)
	if err != nil {
		log.Error(err, "configuring GUE service")
		return done, err
	}

	// Cache some values that we might need later if we need to delete
	// the endpoint
	patchBytes, err := json.Marshal(egwv1.Endpoint{Status: egwv1.EndpointStatus{GUEKey: lb.Spec.GUEKey, ProxyIfindex: lb.Status.ProxyIfindex, TunnelID: gueEp.TunnelID}})
	if err != nil {
		log.Error(err, "marshaling EP patch", "ep", ep)
		return done, err
	}
	err = r.Status().Patch(ctx, ep, client.RawPatch(types.MergePatchType, patchBytes))
	if err != nil {
		log.Error(err, "patching EP status", "ep", ep)
		return done, err
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *EndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.Endpoint{}).
		Complete(r)
}

func (r *EndpointReconciler) configureTunnel(l logr.Logger, ep egwv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel get %[1]d || /opt/acnodal/bin/cli_tunnel set %[1]d %[2]s %[3]d 0 0", ep.TunnelID, ep.Address, ep.Port.Port)
	l.Info(script)
	cmd := exec.Command("/bin/sh", "-c", script)
	return cmd.Run()
}

// setGUEIngressAddress sets the GUEAddress/Port fields of the
// LoadBalancer status. This is used by PureLB to open a GUE tunnel
// back to the EGW. It returns a GUETunnelEndpoint, either the newly
// created one or the existing one. If error is non-nil then the
// GUETunnelEndpoint is invalid.
func (r *EndpointReconciler) setGUEIngressAddress(ctx context.Context, lb *egwv1.LoadBalancer, ep *egwv1.EndpointSpec) (egwv1.GUETunnelEndpoint, error) {
	var (
		err         error
		gueEndpoint egwv1.GUETunnelEndpoint
	)

	// We set up a tunnel to each client node that has an endpoint that
	// belongs to this service. If we already have a tunnel to this
	// endpoint's node (i.e., two endpoints in the same service on the
	// same node) then we don't need to do anything
	if gueEndpoint, exists := lb.Status.GUETunnelEndpoints[ep.NodeAddress]; exists {
		log.Info("EP node already has a tunnel", "endpoint", ep)
		return gueEndpoint, nil
	}

	// fetch the NodeConfig; it tells us the GUEEndpoint for this node
	nc := &egwv1.NodeConfig{}
	err = r.Get(ctx, types.NamespacedName{Name: "default", Namespace: "egw"}, nc)
	if err != nil {
		return gueEndpoint, err
	}
	nc.Spec.Base.GUEEndpoint.DeepCopyInto(&gueEndpoint)
	gueEndpoint.TunnelID = tunnelID

	// FIXME: allocate this instead of just incrementing
	tunnelID++

	// prepare a patch to set this node's tunnel endpoint in the LB status
	patchBytes, err := json.Marshal(egwv1.LoadBalancer{Status: egwv1.LoadBalancerStatus{GUETunnelEndpoints: map[string]egwv1.GUETunnelEndpoint{ep.NodeAddress: gueEndpoint}}})
	if err != nil {
		return gueEndpoint, err
	}
	log.Info(string(patchBytes))

	// apply the patch
	err = r.Status().Patch(ctx, lb, client.RawPatch(types.MergePatchType, patchBytes))
	if err != nil {
		log.Error("patching LB status", "lb", lb, "error", err)
		return gueEndpoint, err
	}

	log.Info("LB status patched", "lb", lb)
	return gueEndpoint, err
}

func (r *EndpointReconciler) deleteTunnel(l logr.Logger, ep egwv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", ep.TunnelID)
	l.Info(script)
	cmd := exec.Command("/bin/sh", "-c", script)
	return cmd.Run()
}

func (r *EndpointReconciler) configureService(l logr.Logger, ep egwv1.EndpointSpec, ifindex int, tunnelID uint32, tunnelKey uint32) error {
	// split the tunnelKey into its parts: groupId in the upper 16 bits
	// and serviceId in the lower 16
	var groupID uint16 = uint16(tunnelKey & 0xffff)
	var serviceID uint16 = uint16(tunnelKey >> 16)

	script := fmt.Sprintf("/opt/acnodal/bin/cli_service set-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	l.Info(script)
	cmd := exec.Command("/bin/sh", "-c", script)
	return cmd.Run()
}

// cleanupService undoes the PFC setup that we did for this Endpoint.
func (r *EndpointReconciler) cleanupService(l logr.Logger, ep egwv1.EndpointSpec, ifindex int, tunnelID uint32, tunnelKey uint32) error {
	// split the tunnelKey into its parts: groupId in the upper 16 bits
	// and serviceId in the lower 16
	var groupID uint16 = uint16(tunnelKey & 0xffff)
	var serviceID uint16 = uint16(tunnelKey >> 16)

	script := fmt.Sprintf("/opt/acnodal/bin/cli_service del-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	l.Info(script)
	cmd := exec.Command("/bin/sh", "-c", script)
	return cmd.Run()
}
