package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

// LoadBalancerCallbacks are how this controller notifies the control
// plane of object changes.
type LoadBalancerCallbacks interface {
	LoadBalancerChanged(*egwv1.LoadBalancer) error
}

// LoadBalancerReconciler reconciles a LoadBalancer object
type LoadBalancerReconciler struct {
	client.Client
	Log       logr.Logger
	Callbacks LoadBalancerCallbacks
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch

// Reconcile is the core of this controller. It gets requests from the
// controller-runtime and figures out what to do with them.
func (r *LoadBalancerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	result := ctrl.Result{}
	ctx := context.TODO()
	_ = r.Log.WithValues("loadbalancer", req.NamespacedName)

	// read the object that caused the event
	lb := &egwv1.LoadBalancer{}
	err := r.Get(ctx, req.NamespacedName, lb)
	if err != nil {
		return result, err
	}

	// tell the control plane about the changed object
	err = r.Callbacks.LoadBalancerChanged(lb)
	if err != nil {
		return result, err
	}

	return result, nil
}

// SetupWithManager sets up this reconciler to be managed.
func (r *LoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.LoadBalancer{}).
		Complete(r)
}
