package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// GWEndpointSliceReconciler reconciles a GWEndpointSlice object
type GWEndpointSliceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GWEndpointSliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWEndpointSlice{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwendpointslices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwendpointslices/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GWEndpointSlice object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *GWEndpointSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	slice := epicv1.GWEndpointSlice{}

	// Get the resource that caused this event.
	if err := r.Get(ctx, req.NamespacedName, &slice); err != nil {
		l.Info("Can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}
	l = l.WithValues("clientName", slice.Spec.ClientRef.Name)

	if !slice.ObjectMeta.DeletionTimestamp.IsZero() {
		// This rep is marked for deletion. We need to clean up where
		// possible, but also be careful to handle edge cases like the LB
		// being "not found" because it was deleted first.

		l.Info("To be deleted")

		// FIXME: need to find the proxy via routes and deal with routes being deleted
		// // Remove the rep's info from the LB but continue even if there's
		// // an error because we want to *always* remove our finalizer.
		// repInfoErr := removeRepInfo(ctx, r.Client, l, req.NamespacedName.Namespace, slice.Labels[epicv1.OwningProxyLabel], rep)
		var repInfoErr error = nil

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		if err := RemoveFinalizer(ctx, r.Client, &slice, epicv1.FinalizerName); err != nil {
			return done, err
		}

		// Now that we've removed our finalizer we can return and report
		// on any errors that happened during cleanup.
		return done, repInfoErr
	}

	// The proxy is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	if err := AddFinalizer(ctx, r.Client, &slice, epicv1.FinalizerName); err != nil {
		return done, err
	}

	l.V(1).Info("Reconciling")

	// Get the proxies that are connected to this slice. It can be more
	// than one since more than one GWRoute and reference this slice,
	// and each GWRoute can reference more than one GWProxy.
	proxies, err := slice.ReferencingProxies(ctx, r.Client)
	if err != nil {
		return done, err
	}

	// Nudge each of our parent proxies.
	for _, proxy := range proxies {
		l.Info("parent", "proxy", proxy.NamespacedName())
		if err := proxy.Nudge(ctx, r.Client, l); err != nil {
			return done, err
		}
	}

	return done, nil
}
