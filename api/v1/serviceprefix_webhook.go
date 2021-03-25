package v1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
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
	ValidatePool(*ServicePrefix) error
}

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *ServicePrefix) SetupWebhookWithManager(mgr ctrl.Manager, val PoolValidator) error {
	spValidator = val
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-epic-acnodal-io-v1-serviceprefix,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=serviceprefixes,versions=v1,name=vserviceprefix.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1

var _ webhook.Validator = &ServicePrefix{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *ServicePrefix) ValidateCreate() error {
	spLog.Info("validate create", "name", r.Name)
	return spValidator.ValidatePool(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *ServicePrefix) ValidateUpdate(old runtime.Object) error {
	spLog.Info("validate update", "name", r.Name)
	return spValidator.ValidatePool(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *ServicePrefix) ValidateDelete() error {
	return nil
}
