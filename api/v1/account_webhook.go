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

// +kubebuilder:webhook:path=/mutate-egw-acnodal-io-v1-account,mutating=true,failurePolicy=fail,groups=egw.acnodal.io,resources=accounts,verbs=create;update,versions=v1,name=maccount.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1

var _ webhook.Defaulter = &Account{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Account) Default() {
	accountlog.Info("default", "name", r.Name)

	// add a fake GUE key to this service
	// FIXME: get this key from the EGW singleton
	r.Spec.GUEKey = 42
}
