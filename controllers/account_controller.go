package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// AccountReconciler reconciles a Account object
type AccountReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=accounts,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=accounts/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *AccountReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}
	ctx := context.Background()
	log := r.Log.WithValues("account", req.NamespacedName)

	log.Info("reconciling")

	// read the object that caused the event
	account := &epicv1.Account{}
	err = r.Get(ctx, req.NamespacedName, account)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("resource not found. Ignoring since object must be deleted")
			return result, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource, will retry")
		return result, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *AccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.Account{}).
		Complete(r)
}
