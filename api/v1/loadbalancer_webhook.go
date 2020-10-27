package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var loadbalancerlog = logf.Log.WithName("loadbalancer-resource")

func (r *LoadBalancer) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-egw-acnodal-io-v1-loadbalancer,mutating=false,failurePolicy=fail,groups=egw.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io

var _ webhook.Validator = &LoadBalancer{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateCreate() error {
	loadbalancerlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateUpdate(old runtime.Object) error {
	loadbalancerlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateDelete() error {
	loadbalancerlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}