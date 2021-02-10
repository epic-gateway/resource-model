package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	"gitlab.com/acnodal/egw-resource-model/internal/pfc"
)

// EGWReconciler reconciles a EGW object
type EGWReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=egws,verbs=get;list;watch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *EGWReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	done := ctrl.Result{Requeue: false}
	ctx := context.Background()
	log := r.Log.WithValues("EGW", req.NamespacedName)

	// read the object that caused the event
	config := &egwv1.EGW{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	base := config.Spec.NodeBase
	for _, nic := range base.IngressNICs {
		if err := pfc.SetupNIC(nic, "decap", "ingress", 0, 9); err != nil {
			log.Error(err, "Failed to setup NIC "+nic)
		}
		if err := pfc.SetupNIC(nic, "encap", "egress", 1, 25); err != nil {
			log.Error(err, "Failed to setup NIC "+nic)
		}
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *EGWReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.EGW{}).
		Complete(r)
}
