package v1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var epiclog = logf.Log.WithName("epic-resource")

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *EPIC) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-epic-acnodal-io-v1-epic,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=epics,verbs=create;update,versions=v1,name=mepic.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Defaulter = &EPIC{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *EPIC) Default() {
	epiclog.Info("default", "name", r.Name)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-epic-acnodal-io-v1-epic,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=epics,versions=v1,name=vepic.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Validator = &EPIC{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *EPIC) ValidateCreate() error {
	epiclog.Info("validate create", "name", r.Name)

	// The name and namespace must both be "epic"
	if r.Name != ConfigName {
		return fmt.Errorf("the EPIC object's name must be \"%s\"", ConfigName)
	}
	if r.Namespace != ConfigNamespace {
		return fmt.Errorf("the EPIC object must be in the \"%s\" namespace", ConfigNamespace)
	}

	// THERE CAN BE ONLY ONE - k8s enforces that each object's name must
	// be unique within its GVK and namespace so since we're rejecting
	// all but "epic/epic" we don't need to do anything here

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *EPIC) ValidateUpdate(old runtime.Object) error {
	epiclog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *EPIC) ValidateDelete() error {
	epiclog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
