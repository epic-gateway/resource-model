package v1

import (
	"context"
	"fmt"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var (
	lbLog     = logf.Log.WithName("loadbalancer-resource")
	allocator PoolAllocator
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *LoadBalancer) SetupWebhookWithManager(mgr ctrl.Manager, alloc PoolAllocator) error {
	allocator = alloc
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=accounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=accounts/status,verbs=get;update

// +kubebuilder:webhook:verbs=create,path=/mutate-epic-acnodal-io-v1-loadbalancer,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Defaulter = &LoadBalancer{}

// Default sets default values for this LoadBalancer object.
func (r *LoadBalancer) Default() {
	ctx := context.TODO()
	lbLog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// LoadBalancer is deleted.
	controllerutil.AddFinalizer(r, FinalizerName)

	// Fetch this LB's owning service group
	sg, err := r.getLBServiceGroup(ctx, crtclient)
	if err != nil {
		lbLog.Error(err, "failed to fetch owning service group")
		return
	}

	// Get the name of the ServicePrefix - it's also the name of the
	// address pool
	poolName := sg.Labels[OwningServicePrefixLabel]

	// If the user hasn't provided an Envoy config template or replica
	// count, then copy them from the owning LBServiceGroup
	if r.Spec.EnvoyTemplate == (*marin3r.EnvoyConfigSpec)(nil) {
		r.Spec.EnvoyTemplate = &sg.Spec.EnvoyTemplate
	}
	if r.Spec.EnvoyReplicaCount == nil || *r.Spec.EnvoyReplicaCount < 1 {
		r.Spec.EnvoyReplicaCount = pointer.Int32Ptr(sg.Spec.EnvoyReplicaCount)
	}

	// Add an external address if needed
	if r.Spec.PublicAddress == "" {
		address, err := allocator.AllocateFromPool(r.NamespacedName().String(), poolName, r.Spec.PublicPorts, "")
		if err != nil {
			lbLog.Error(err, "allocation failed")
			return
		}
		r.Spec.PublicAddress = address.String()

		// add an Endpoint struct for external-dns
		r.AddDNSEndpoint(*sg)
	}

	lbLog.Info("defaulted", "name", r.Name, "contents", r)
}

// +kubebuilder:webhook:verbs=create;delete,path=/validate-epic-acnodal-io-v1-loadbalancer,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Validator = &LoadBalancer{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateCreate() error {
	ctx := context.TODO()
	lbLog.Info("validate create", "name", r.Name, "contents", r)

	// Block create if there's no owning SG
	sg, err := r.getLBServiceGroup(ctx, crtclient)
	if err != nil {
		return err
	}

	// Block create if there's no owning account
	if _, err := sg.getAccount(ctx, crtclient); err != nil {
		return err
	}

	// Block create if the IP allocation failed
	if r.Spec.PublicAddress == "" {
		return fmt.Errorf("%s/%s IP address allocation failed", r.Namespace, r.Name)
	}

	return nil
}

// ValidateUpdate does nothing.
func (r *LoadBalancer) ValidateUpdate(old runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateDelete() error {
	lbLog.Info("validate delete", "name", r.Name)

	// Don't allow deletion if there are any upstream clusters still
	// using this LB
	if len(r.Spec.UpstreamClusters) > 0 {
		return fmt.Errorf("LB %s has upstream clusters, can't delete: %v", r.NamespacedName(), r.Spec.UpstreamClusters)
	}

	return nil
}
