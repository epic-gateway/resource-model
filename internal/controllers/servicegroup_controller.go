package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

// ServiceGroupReconciler reconciles a ServiceGroup object
type ServiceGroupReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=servicegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=servicegroups/status,verbs=get;update;patch

// Reconcile is the core of this controller. It gets requests from the
// controller-runtime and figures out what to do with them.
func (r *ServiceGroupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = r.Log.WithValues("servicegroup", req.NamespacedName)

	// your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up this reconciler to be managed.
func (r *ServiceGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.ServiceGroup{}).
		Complete(r)
}
