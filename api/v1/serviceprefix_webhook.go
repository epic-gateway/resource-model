package v1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var (
	spLog       = logf.Log.WithName("serviceprefix-resource")
	spValidator PoolValidator
)

// PoolValidator validates incoming ServicePrefixes. We use an
// interface to avoid import loops between the v1 package and the
// allocator package.
// +kubebuilder:object:generate=false
type PoolValidator interface {
	ValidateCreate(*ServicePrefix) error
	ValidateUpdate(*ServicePrefix) error
	ValidateDelete(*ServicePrefix) error
}

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *ServicePrefix) SetupWebhookWithManager(mgr ctrl.Manager, val PoolValidator) error {
	spValidator = val
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
}

// +kubebuilder:webhook:verbs=create,path=/mutate-epic-acnodal-io-v1-serviceprefix,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=serviceprefixes,versions=v1,name=vserviceprefix.kb.io,sideEffects=None,admissionReviewVersions=v1

// Default sets default values for this LoadBalancer object.
func (r *ServicePrefix) Default() {
	spLog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// LoadBalancer is deleted.
	r.Finalizers = append(r.Finalizers, ServicePrefixFinalizerName)
}
