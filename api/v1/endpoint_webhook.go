package v1

import (
	"context"
	"fmt"
	"net"
	"reflect"

	"k8s.io/apimachinery/pkg/labels"
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

	lbname := types.NamespacedName{Namespace: r.ObjectMeta.Namespace, Name: r.Spec.LoadBalancer}
	// add a label with the owning LB's name to make it easier to query
	if r.Labels == nil {
		r.Labels = map[string]string{}
	}
	r.Labels[OwningLoadBalancerLabel] = lbname.Name
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

	// Parameter validation
	if net.ParseIP(r.Spec.Address) == nil {
		return fmt.Errorf("bad input: can't parse address \"%s\"", r.Spec.Address)
	}
	if net.ParseIP(r.Spec.NodeAddress) == nil {
		return fmt.Errorf("bad input: can't parse node-address \"%s\"", r.Spec.NodeAddress)
	}

	// Block create if there's no owning LB
	lb := &LoadBalancer{}
	lbname := types.NamespacedName{Namespace: r.ObjectMeta.Namespace, Name: r.Spec.LoadBalancer}
	if err := crtclient.Get(context.TODO(), lbname, lb); err != nil {
		endpointlog.Info("bad input: no owning load balancer", "name", lbname)
		return err
	}

	// Block create if this loadbalancer has a duplicate endpoint
	labelSelector := labels.SelectorFromSet(map[string]string{OwningLoadBalancerLabel: r.Spec.LoadBalancer})
	listOps := client.ListOptions{Namespace: r.ObjectMeta.Namespace, LabelSelector: labelSelector}
	list := EndpointList{}
	if err := crtclient.List(context.TODO(), &list, &listOps); err != nil {
		endpointlog.Error(err, "Listing endpoints", "name", lbname)
		return err
	}
	for _, ep := range list.Items {
		if reflect.DeepEqual(r.Spec, ep.Spec) {
			return fmt.Errorf("duplicate endpoint: %s", ep.Name)
		}
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
