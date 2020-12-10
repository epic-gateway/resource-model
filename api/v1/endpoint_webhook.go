package v1

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	// log is for logging in this package.
	endpointlog = logf.Log.WithName("endpoint-resource")

	// crtclient looks up objects related to the one we're defaulting.
	crtclient client.Client

	// epscheme is used when we set the LB endpoint owner.
	epscheme *runtime.Scheme
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *Endpoint) SetupWebhookWithManager(mgr ctrl.Manager) error {
	crtclient = mgr.GetClient()
	epscheme = mgr.GetScheme()

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-egw-acnodal-io-v1-endpoint,mutating=true,failurePolicy=fail,groups=egw.acnodal.io,resources=endpoints,verbs=create,versions=v1,name=mendpoint.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Defaulter = &Endpoint{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Endpoint) Default() {
	endpointlog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// Endpoint is deleted.
	r.ObjectMeta.Finalizers = append(r.ObjectMeta.Finalizers, EndpointFinalizerName)

	// Set this EP's owning LB so it will get deleted if the LB does
	if len(r.ObjectMeta.OwnerReferences) == 0 {
		lb := &LoadBalancer{}
		lbname := types.NamespacedName{Namespace: r.ObjectMeta.Namespace, Name: r.Spec.LoadBalancer}
		if err := crtclient.Get(context.TODO(), lbname, lb); err != nil {
			endpointlog.Error(err, "Failed to find owning load balancer", "name", lbname)
			return
		}

		ctrl.SetControllerReference(lb, r, epscheme)
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-egw-acnodal-io-v1-endpoint,mutating=false,failurePolicy=fail,groups=egw.acnodal.io,resources=endpoints,versions=v1,name=vendpoint.kb.io,sideEffects=none,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Validator = &Endpoint{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Endpoint) ValidateCreate() error {
	endpointlog.Info("validate create", "name", r.Name)

	// Block create if there's no owning LB
	lb := &LoadBalancer{}
	lbname := types.NamespacedName{Namespace: r.ObjectMeta.Namespace, Name: r.Spec.LoadBalancer}
	if err := crtclient.Get(context.TODO(), lbname, lb); err != nil {
		endpointlog.Error(err, "No owning load balancer", "name", lbname)
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Endpoint) ValidateUpdate(old runtime.Object) error {
	endpointlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Endpoint) ValidateDelete() error {
	endpointlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}