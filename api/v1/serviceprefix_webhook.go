package v1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// log is for logging in this package.
var (
	spLog = logf.Log.WithName("serviceprefix-resource")
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *ServicePrefix) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
}

// +kubebuilder:webhook:verbs=create,path=/mutate-epic-acnodal-io-v1-serviceprefix,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=serviceprefixes,versions=v1,name=vserviceprefix.kb.io,sideEffects=None,admissionReviewVersions=v1

// Default sets default values for this object.
func (r *ServicePrefix) Default() {
	spLog.Info("default", "prefixName", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// object is deleted.
	r.Finalizers = append(r.Finalizers, FinalizerName)
}
