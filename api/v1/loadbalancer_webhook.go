package v1

import (
	"context"
	"fmt"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var loadbalancerlog = logf.Log.WithName("loadbalancer-resource")

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *LoadBalancer) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=accounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=accounts/status,verbs=get;update

// +kubebuilder:webhook:verbs=create,path=/mutate-egw-acnodal-io-v1-loadbalancer,mutating=true,failurePolicy=fail,groups=egw.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Defaulter = &LoadBalancer{}

// Default sets default values for this LoadBalancer object.
func (r *LoadBalancer) Default() {
	ctx := context.TODO()
	loadbalancerlog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// LoadBalancer is deleted.
	r.Finalizers = append(r.Finalizers, LoadbalancerFinalizerName)

	// add a service id to this service
	serviceID, err := allocateServiceID(ctx, crtclient, r.Namespace, r.Labels[OwningServiceGroupLabel])
	if err != nil {
		loadbalancerlog.Info("failed to allocate serviceID", "error", err)
	}
	r.Spec.ServiceID = serviceID

	// If the user hasn't provided an Envoy config template, copy it
	// from the owning ServiceGroup
	if r.Spec.EnvoyTemplate == (*marin3r.EnvoyConfigSpec)(nil) {
		sg, err := r.fetchServiceGroup()
		if err != nil {
			loadbalancerlog.Info("failed to fetch owning service group", "error", err)
			return
		}
		r.Spec.EnvoyTemplate = &sg.Spec.EnvoyTemplate
	}
	loadbalancerlog.Info("defaulted", "name", r.Name, "contents", r)
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-egw-acnodal-io-v1-loadbalancer,mutating=false,failurePolicy=fail,groups=egw.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,sideEffects=none,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Validator = &LoadBalancer{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateCreate() error {
	loadbalancerlog.Info("validate create", "name", r.Name, "contents", r)

	// Block create if there's no owning SG
	if _, err := r.fetchServiceGroup(); err != nil {
		return err
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateUpdate(old runtime.Object) error {
	loadbalancerlog.Info("validate update", "name", r.Name, "old", old, "new", r)
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateDelete() error {
	loadbalancerlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

func (r *LoadBalancer) fetchServiceGroup() (*ServiceGroup, error) {
	// Block create if there's no owning SG
	if r.Labels[OwningServiceGroupLabel] == "" {
		return nil, fmt.Errorf("LB has no owning service group label")
	}
	sgName := types.NamespacedName{Namespace: r.Namespace, Name: r.Labels[OwningServiceGroupLabel]}
	sg := &ServiceGroup{}
	if err := crtclient.Get(context.TODO(), sgName, sg); err != nil {
		replog.Info("bad input: no owning service group", "name", sgName)
		return nil, err
	}

	return sg, nil
}

// allocateServiceID allocates a service id from the Account that owns
// this LB. If this call succeeds (i.e., error is nil) then the
// returned service id will be unique.
func allocateServiceID(ctx context.Context, cl client.Client, acctNS string, acctName string) (serviceID uint16, err error) {
	tries := 3
	for err = fmt.Errorf(""); err != nil && tries > 0; tries-- {
		serviceID, err = nextServiceID(ctx, cl, acctNS, acctName)
		if err != nil {
			accountlog.Info("problem allocating account serviceID", "error", err)
		}
	}
	return serviceID, err
}

// nextServiceID gets the next LB ServiceID by doing a
// read-modify-write cycle. It might be inefficient in terms of not
// using all of the values that it allocates but it's safe because the
// Update() will only succeed if the Account hasn't been modified
// since the Get().
//
// This function doesn't retry so if there's a collision with some
// other process the caller needs to retry.
func nextServiceID(ctx context.Context, cl client.Client, acctNS string, acctName string) (serviceID uint16, err error) {

	// load this LBs owning Account which holds the ServiceID allocator
	accountName := types.NamespacedName{Namespace: acctNS, Name: acctName}
	account := Account{}
	if err := cl.Get(ctx, accountName, &account); err != nil {
		loadbalancerlog.Info("can't Get owning Account", "error", err)
		return 0, err
	}

	// increment the Account's service ID counter
	account.Status.CurrentServiceID++

	return account.Status.CurrentServiceID, cl.Status().Update(ctx, &account)
}
