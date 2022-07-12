package v1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var accountlog = logf.Log.WithName("account-resource")

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *Account) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
}

// +kubebuilder:webhook:path=/mutate-epic-acnodal-io-v1-account,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=accounts,verbs=create,versions=v1,name=maccount.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Defaulter = &Account{}

// Default sets default values for this Account object.
func (r *Account) Default() {
}
