package v1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var egwlog = logf.Log.WithName("egw-resource")

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *EGW) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-egw-acnodal-io-v1-egw,mutating=true,failurePolicy=fail,groups=egw.acnodal.io,resources=egws,verbs=create;update,versions=v1,name=megw.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Defaulter = &EGW{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *EGW) Default() {
	egwlog.Info("default", "name", r.Name)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-egw-acnodal-io-v1-egw,mutating=false,failurePolicy=fail,groups=egw.acnodal.io,resources=egws,versions=v1,name=vegw.kb.io,sideEffects=none,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Validator = &EGW{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *EGW) ValidateCreate() error {
	egwlog.Info("validate create", "name", r.Name)

	// The name and namespace must both be "egw"
	if r.Name != "egw" {
		return fmt.Errorf("the EGW object's name must be \"egw\"")
	}
	if r.Namespace != ConfigNamespace {
		return fmt.Errorf("the EGW object must be in the \"egw\" namespace")
	}

	// THERE CAN BE ONLY ONE - k8s enforces that each object's name must
	// be unique within its GVK and namespace so since we're rejecting
	// all but "egw/egw" we don't need to do anything here

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *EGW) ValidateUpdate(old runtime.Object) error {
	egwlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *EGW) ValidateDelete() error {
	egwlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
