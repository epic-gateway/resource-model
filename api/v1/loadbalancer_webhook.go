package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var loadbalancerlog = logf.Log.WithName("loadbalancer-resource")

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *LoadBalancer) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create,path=/mutate-egw-acnodal-io-v1-loadbalancer,mutating=true,failurePolicy=fail,groups=egw.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Defaulter = &LoadBalancer{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LoadBalancer) Default() {
	loadbalancerlog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// LoadBalancer is deleted.
	r.ObjectMeta.Finalizers = append(r.ObjectMeta.Finalizers, LoadbalancerFinalizerName)

	// add a GUE key to this service. we're pretending that the account
	// key is 42 and the service key is 42
	r.Spec.GUEKey = (42 * 0x10000) + 42
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-egw-acnodal-io-v1-loadbalancer,mutating=false,failurePolicy=fail,groups=egw.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,sideEffects=none,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Validator = &LoadBalancer{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateCreate() error {
	loadbalancerlog.Info("validate create", "name", r.Name, "contents", r)
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateUpdate(old runtime.Object) error {
	loadbalancerlog.Info("validate update", "name", r.Name, "old", old, "new", r)
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateDelete() error {
	loadbalancerlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
