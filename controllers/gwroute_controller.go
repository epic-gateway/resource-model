package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// GWRouteReconciler reconciles a GWRoute object
type GWRouteReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GWRoute object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *GWRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := r.Log.WithValues("gwroute", req.NamespacedName)
	route := epicv1.GWRoute{}

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		l.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	l = l.WithValues("version", route.ResourceVersion)

	l.Info("reconciling")

	return done, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GWRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWRoute{}).
		Complete(r)
}
