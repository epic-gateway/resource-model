package controllers

import (
	"context"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/envoy"
)

// RemoteEndpointReconciler reconciles RemoteEndpoint objects.
type RemoteEndpointReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *RemoteEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("rep", req.NamespacedName)
	rep := &epicv1.RemoteEndpoint{}
	lb := &epicv1.LoadBalancer{}
	ec := &marin3r.EnvoyConfig{}

	// Get the RemoteEndpoint that caused the event
	if err := r.Get(ctx, req.NamespacedName, rep); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Check if k8s wants to delete this RemoteEndpoint
	if !rep.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("object to be deleted")

		if controllerutil.ContainsFinalizer(rep, epicv1.RemoteEndpointFinalizerName) {
			// remove our finalizer from the list and update the object
			controllerutil.RemoveFinalizer(rep, epicv1.RemoteEndpointFinalizerName)
			if err := r.Update(ctx, rep); err != nil {
				return done, err
			}
		}
	}

	// Get the LoadBalancer that owns this RemoteEndpoint
	lbName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: rep.Labels[epicv1.OwningLoadBalancerLabel]}
	if err := r.Get(ctx, lbName, lb); err != nil {
		return done, err
	}

	// Update the Marin3r EnvoyConfig with the current set of active
	// endpoints

	// list the endpoints that belong to the parent LB
	reps, err := listActiveLBEndpoints(ctx, r, lb)
	if err != nil {
		return done, err
	}

	// Get the EnvoyConfig
	ecName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Name}
	if err := r.Get(ctx, ecName, ec); err != nil {
		return done, err
	}

	log.Info("updating endpoints", "endpoints", reps)

	// FIXME: instead of updating the cluster can we just "nudge" the LB
	// CR which will trigger the LBController to to the same thing?

	// Update the EnvoyConfig's Cluster which contains the endpoints
	cluster, err := envoy.ServiceToCluster(*lb, reps)
	if err != nil {
		return done, err
	}
	ec.Spec.EnvoyResources.Clusters = cluster
	if err = r.Update(ctx, ec); err != nil {
		return done, err
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *RemoteEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.RemoteEndpoint{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *RemoteEndpointReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// listActiveLBEndpoints lists the endpoints that belong to lb that
// are active, i.e., not in the process of being deleted.
func listActiveLBEndpoints(ctx context.Context, cl client.Client, lb *epicv1.LoadBalancer) ([]epicv1.RemoteEndpoint, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{epicv1.OwningLoadBalancerLabel: lb.Name})
	listOps := client.ListOptions{Namespace: lb.Namespace, LabelSelector: labelSelector}
	list := epicv1.RemoteEndpointList{}
	err := cl.List(ctx, &list, &listOps)

	activeEPs := []epicv1.RemoteEndpoint{}
	// build a new list with no "in deletion" endpoints
	for _, endpoint := range list.Items {
		if endpoint.ObjectMeta.DeletionTimestamp.IsZero() {
			activeEPs = append(activeEPs, endpoint)
		}
	}

	return activeEPs, err
}
