package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/pfc"
)

// EPICAgentReconciler reconciles a EPIC object
type EPICAgentReconciler struct {
	client.Client
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=epics,verbs=get;list;watch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *EPICAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// read the object that caused the event
	config := &epicv1.EPIC{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		l.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	base := config.Spec.NodeBase
	for _, nic := range base.IngressNICs {
		if err := pfc.SetupNIC(nic, "decap", "ingress", 0, 9); err != nil {
			l.Error(err, "Failed to setup NIC "+nic)
		}
		if err := pfc.SetupNIC(nic, "encap", "egress", 1, 25); err != nil {
			l.Error(err, "Failed to setup NIC "+nic)
		}
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *EPICAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.EPIC{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *EPICAgentReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}
