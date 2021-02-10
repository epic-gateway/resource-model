package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	"gitlab.com/acnodal/egw-resource-model/internal/envoy"
	egwexec "gitlab.com/acnodal/egw-resource-model/internal/exec"
)

// RemoteEndpointReconciler reconciles RemoteEndpoint objects.
type RemoteEndpointReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=remoteendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=remoteendpoints/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *RemoteEndpointReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	done := ctrl.Result{Requeue: false}
	tryAgain := ctrl.Result{RequeueAfter: 10 * time.Second}
	ctx := context.Background()
	log := r.Log.WithValues("rep", req.NamespacedName)

	// Get the RemoteEndpoint that caused the event
	rep := &egwv1.RemoteEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, rep); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Get the LoadBalancer that owns this RemoteEndpoint
	lb := &egwv1.LoadBalancer{}
	lbname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: rep.Labels[egwv1.OwningLoadBalancerLabel]}
	if err := r.Get(ctx, lbname, lb); err != nil {
		log.Error(err, "Failed to find owning load balancer", "name", lbname)
		return done, err
	}

	// Get the ServiceGroup that owns this RemoteEndpoint
	sg := &egwv1.ServiceGroup{}
	sgname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Labels[egwv1.OwningServiceGroupLabel]}
	if err := r.Get(ctx, sgname, sg); err != nil {
		log.Error(err, "Failed to find owning service group", "name", sgname)
		return done, err
	}

	// get the Account that owns this LB
	account := &egwv1.Account{}
	accountNSName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: sg.Labels[egwv1.OwningAccountLabel]}
	if err := r.Get(ctx, accountNSName, account); err != nil {
		return done, err
	}

	// Check if k8s wants to delete this RemoteEndpoint
	if !rep.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(rep, egwv1.RemoteEndpointFinalizerName) {
			log.Info("object to be deleted")

			if err := r.cleanupService(log, rep.Spec, rep.Status.ProxyIfindex, rep.Status.TunnelID, rep.Status.GroupID, rep.Status.ServiceID); err != nil {
				log.Error(err, "Failed to cleanup PFC")
			}

			// remove our finalizer from the list and update the object
			controllerutil.RemoveFinalizer(rep, egwv1.RemoteEndpointFinalizerName)
			if err := r.Update(ctx, rep); err != nil {
				return done, err
			}
		}

	} else {

		// The endpoint isn't being deleted, so set up a tunnel to it

		// If the LB doesn't have its ifindex set then we can't configure,
		// so try again in a few seconds
		if lb.Status.ProxyIfindex == 0 {
			log.Info("LB ifindex not set yet")
			return tryAgain, nil
		}

		// Add GUE ingress address/port to the endpoint
		gueEp, err := r.setGUEIngressAddress(ctx, log, lb, &rep.Spec)
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
		err = r.configureService(log, rep.Spec, lb.Status.ProxyIfindex, account.Spec.GroupID, lb.Spec.ServiceID, gueEp.TunnelID, sg.Spec.AuthCreds)
		if err != nil {
			log.Error(err, "configuring GUE service")
			return done, err
		}

		// Cache some values that we might need later if we need to delete
		// the endpoint
		patchBytes, err := json.Marshal(egwv1.RemoteEndpoint{Status: egwv1.RemoteEndpointStatus{GroupID: account.Spec.GroupID, ServiceID: lb.Spec.ServiceID, ProxyIfindex: lb.Status.ProxyIfindex, TunnelID: gueEp.TunnelID}})
		if err != nil {
			log.Error(err, "marshaling EP patch", "ep", rep)
			return done, err
		}
		err = r.Status().Patch(ctx, rep, client.RawPatch(types.MergePatchType, patchBytes))
		if err != nil {
			log.Error(err, "patching EP status", "ep", rep)
			return done, err
		}
	}

	// Update the Marin3r EnvoyConfig with the current set of active
	// endpoints

	// list the endpoints that belong to the parent LB
	reps, err := listActiveLBEndpoints(ctx, r, lb)
	if err != nil {
		return done, err
	}

	// Get the EnvoyConfig
	ec := marin3r.EnvoyConfig{}
	ecname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Name}
	if err := r.Get(ctx, ecname, &ec); err != nil {
		log.Error(err, "Failed to find envoy config", "name", ecname)
		return done, err
	}

	// Update the EnvoyConfig's Cluster which contains the endpoints
	cluster, err := envoy.ServiceToCluster(*lb, reps)
	if err != nil {
		return done, err
	}
	ec.Spec.EnvoyResources.Clusters = cluster
	if err = r.Update(ctx, &ec); err != nil {
		return done, err
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *RemoteEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.RemoteEndpoint{}).
		Complete(r)
}

func (r *RemoteEndpointReconciler) configureTunnel(l logr.Logger, ep egwv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel get %[1]d || /opt/acnodal/bin/cli_tunnel set %[1]d %[2]s %[3]d 0 0", ep.TunnelID, ep.Address, ep.Port.Port)
	return egwexec.RunScript(l, script)
}

// setGUEIngressAddress sets the GUEAddress/Port fields of the
// LoadBalancer status. This is used by PureLB to open a GUE tunnel
// back to the EGW. It returns a GUETunnelRemoteEndpoint, either the newly
// created one or the existing one. If error is non-nil then the
// GUETunnelRemoteEndpoint is invalid.
func (r *RemoteEndpointReconciler) setGUEIngressAddress(ctx context.Context, l logr.Logger, lb *egwv1.LoadBalancer, ep *egwv1.RemoteEndpointSpec) (egwv1.GUETunnelEndpoint, error) {
	var (
		err         error
		gueEndpoint egwv1.GUETunnelEndpoint
	)

	// We set up a tunnel to each client node that has an endpoint that
	// belongs to this service. If we already have a tunnel to this
	// endpoint's node (i.e., two endpoints in the same service on the
	// same node) then we don't need to do anything
	if gueEndpoint, exists := lb.Status.GUETunnelEndpoints[ep.NodeAddress]; exists {
		l.Info("EP node already has a tunnel", "endpoint", ep)
		return gueEndpoint, nil
	}

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &egwv1.EGW{}
	err = r.Get(ctx, types.NamespacedName{Name: egwv1.ConfigName, Namespace: egwv1.ConfigNamespace}, config)
	if err != nil {
		return gueEndpoint, err
	}
	config.Spec.NodeBase.GUEEndpoint.DeepCopyInto(&gueEndpoint)
	gueEndpoint.TunnelID, err = allocateTunnelID(ctx, l, r.Client)
	if err != nil {
		return gueEndpoint, err
	}

	// prepare a patch to set this node's tunnel endpoint in the LB status
	patchBytes, err := json.Marshal(egwv1.LoadBalancer{Status: egwv1.LoadBalancerStatus{GUETunnelEndpoints: map[string]egwv1.GUETunnelEndpoint{ep.NodeAddress: gueEndpoint}}})
	if err != nil {
		return gueEndpoint, err
	}
	l.Info(string(patchBytes))

	// apply the patch
	err = r.Status().Patch(ctx, lb, client.RawPatch(types.MergePatchType, patchBytes))
	if err != nil {
		l.Info("patching LB status", "lb", lb, "error", err)
		return gueEndpoint, err
	}

	l.Info("LB status patched", "lb", lb)
	return gueEndpoint, err
}

func (r *RemoteEndpointReconciler) deleteTunnel(l logr.Logger, ep egwv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", ep.TunnelID)
	return egwexec.RunScript(l, script)
}

func (r *RemoteEndpointReconciler) configureService(l logr.Logger, ep egwv1.RemoteEndpointSpec, ifindex int, groupID uint16, serviceID uint16, tunnelID uint32, tunnelAuth string) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service set-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	return egwexec.RunScript(l, script)
}

// cleanupService undoes the PFC setup that we did for this RemoteEndpoint.
func (r *RemoteEndpointReconciler) cleanupService(l logr.Logger, ep egwv1.RemoteEndpointSpec, ifindex int, tunnelID uint32, groupID uint16, serviceID uint16) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service del-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, "unused", tunnelID, ep.Address, ep.Port.Port, ifindex)
	return egwexec.RunScript(l, script)
}

// allocateTunnelID allocates a tunnel ID from the EGW singleton. If
// this call succeeds (i.e., error is nil) then the returned ID will
// be unique.
func allocateTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {
	tries := 3
	for err = fmt.Errorf(""); err != nil && tries > 0; tries-- {
		tunnelID, err = nextTunnelID(ctx, l, cl)
		if err != nil {
			l.Info("problem allocating tunnel ID", "error", err)
		}
	}
	return tunnelID, err
}

// nextTunnelID gets the next tunnel ID from the Account CR by doing a
// read-modify-write cycle. It might be inefficient in terms of not
// using all of the values that it allocates but it's safe because the
// Update() will only succeed if the EGW hasn't been modified since
// the Get().
//
// This function doesn't retry so if there's a collision with some
// other process the caller needs to retry.
func nextTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {

	// get the EGW singleton
	egw := egwv1.EGW{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: egwv1.ConfigNamespace, Name: egwv1.ConfigName}, &egw); err != nil {
		l.Info("EGW get failed", "error", err)
		return 0, err
	}

	// increment the EGW's tunnelID counter
	egw.Status.CurrentTunnelID++

	l.Info("allocating tunnelID", "egw", egw, "tunnelID", egw.Status.CurrentTunnelID)

	return egw.Status.CurrentTunnelID, cl.Status().Update(ctx, &egw)
}

// listActiveLBEndpoints lists the endpoints that belong to lb that
// are active, i.e., not in the process of being deleted.
func listActiveLBEndpoints(ctx context.Context, cl client.Client, lb *egwv1.LoadBalancer) ([]egwv1.RemoteEndpoint, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{egwv1.OwningLoadBalancerLabel: lb.Name})
	listOps := client.ListOptions{Namespace: lb.Namespace, LabelSelector: labelSelector}
	list := egwv1.RemoteEndpointList{}
	err := cl.List(ctx, &list, &listOps)

	activeEPs := []egwv1.RemoteEndpoint{}
	// build a new list with no "in deletion" endpoints
	for _, endpoint := range list.Items {
		if endpoint.ObjectMeta.DeletionTimestamp.IsZero() {
			activeEPs = append(activeEPs, endpoint)
		}
	}

	return activeEPs, err
}
