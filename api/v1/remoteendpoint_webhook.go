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

	// epscheme is used when we set the LB endpoint owner.
	epscheme *runtime.Scheme
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *RemoteEndpoint) SetupWebhookWithManager(mgr ctrl.Manager) error {
	crtclient = mgr.GetClient()
	epscheme = mgr.GetScheme()

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-epic-acnodal-io-v1-remoteendpoint,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=remoteendpoints,verbs=create,versions=v1,name=mremoteendpoint.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Defaulter = &RemoteEndpoint{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *RemoteEndpoint) Default() {
	replog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// RemoteEndpoint is deleted.
	r.ObjectMeta.Finalizers = append(r.ObjectMeta.Finalizers, RemoteEndpointFinalizerName)

	// Add the owning LB as a reference so this EP will be deleted if
	// the LB is
	lbname := types.NamespacedName{Namespace: r.ObjectMeta.Namespace, Name: r.ObjectMeta.Labels[OwningLoadBalancerLabel]}
	lb := &LoadBalancer{}
	if err := crtclient.Get(context.TODO(), lbname, lb); err == nil {
		ctrl.SetControllerReference(lb, r, epscheme)
	} else {
		replog.Info("can't find owning load balancer", "name", lbname)
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:verbs=create,path=/validate-epic-acnodal-io-v1-remoteendpoint,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=remoteendpoints,versions=v1,name=vremoteendpoint.kb.io,sideEffects=None,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Validator = &RemoteEndpoint{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *RemoteEndpoint) ValidateCreate() error {
	replog.Info("validate create", "name", r.Name)

	// Parameter validation
	if net.ParseIP(r.Spec.Address) == nil {
		return fmt.Errorf("bad input: can't parse address \"%s\"", r.Spec.Address)
	}
	if r.Spec.NodeAddress != "" && net.ParseIP(r.Spec.NodeAddress) == nil {
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

// ValidateUpdate does nothing.
func (r *RemoteEndpoint) ValidateUpdate(old runtime.Object) error {
	return nil
}

// ValidateDelete does nothing.
func (r *RemoteEndpoint) ValidateDelete() error {
	return nil
}
