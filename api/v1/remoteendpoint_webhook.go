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
	replog = logf.Log.WithName("rep-resource")

	// crtclient looks up objects related to the one we're defaulting.
	crtclient client.Client
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *RemoteEndpoint) SetupWebhookWithManager(mgr ctrl.Manager) error {
	crtclient = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-epic-acnodal-io-v1-remoteendpoint,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=remoteendpoints,verbs=create,versions=v1,name=mremoteendpoint.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Defaulter = &RemoteEndpoint{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *RemoteEndpoint) Default() {
	replog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// RemoteEndpoint is deleted.
	r.ObjectMeta.Finalizers = append(r.ObjectMeta.Finalizers, RemoteEndpointFinalizerName)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create;update,path=/validate-epic-acnodal-io-v1-remoteendpoint,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=remoteendpoints,versions=v1,name=vremoteendpoint.kb.io,sideEffects=none,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Validator = &RemoteEndpoint{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteEndpoint) ValidateCreate() error {
	replog.Info("validate create", "name", r.Name)

	// Parameter validation
	if net.ParseIP(r.Spec.Address) == nil {
		return fmt.Errorf("bad input: can't parse address \"%s\"", r.Spec.Address)
	}
	if net.ParseIP(r.Spec.NodeAddress) == nil {
		return fmt.Errorf("bad input: can't parse node-address \"%s\"", r.Spec.NodeAddress)
	}

	// Block create if there's no owning LB
	ownerName := types.NamespacedName{Namespace: r.ObjectMeta.Namespace, Name: r.Labels[OwningLoadBalancerLabel]}
	owner := &LoadBalancer{}
	if err := crtclient.Get(context.TODO(), ownerName, owner); err != nil {
		replog.Info("bad input: no owning load balancer", "name", ownerName)
		return err
	}

	// Block create if this loadbalancer has a duplicate endpoint
	labelSelector := labels.SelectorFromSet(map[string]string{OwningLoadBalancerLabel: ownerName.Name})
	listOps := client.ListOptions{Namespace: r.ObjectMeta.Namespace, LabelSelector: labelSelector}
	list := RemoteEndpointList{}
	if err := crtclient.List(context.TODO(), &list, &listOps); err != nil {
		replog.Error(err, "Listing endpoints", "name", ownerName)
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
func (r *RemoteEndpoint) ValidateUpdate(old runtime.Object) error {
	replog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteEndpoint) ValidateDelete() error {
	replog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
