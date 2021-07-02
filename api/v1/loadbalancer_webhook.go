package v1

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var (
	loadbalancerlog = logf.Log.WithName("loadbalancer-resource")
	allocator       PoolAllocator
)

// PoolAllocator allocates addresses. We use an interface to avoid
// import loops between the v1 package and the allocator package.
//
// +kubebuilder:object:generate=false
type PoolAllocator interface {
	AllocateFromPool(string, string, []corev1.ServicePort, string) (net.IP, error)
}

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
	loadbalancerlog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// LoadBalancer is deleted.
	controllerutil.AddFinalizer(r, LoadbalancerFinalizerName)

	// Fetch this LB's owning service group
	sg, err := r.fetchLBServiceGroup()
	if err != nil {
		loadbalancerlog.Info("failed to fetch owning service group", "error", err)
		return
	}

	// Get the name of the ServicePrefix - it's also the name of the
	// address pool
	poolName := sg.Labels[OwningServicePrefixLabel]

	account := &Account{}
	accountName := types.NamespacedName{Namespace: sg.Namespace, Name: sg.Labels[OwningAccountLabel]}
	if err := crtclient.Get(ctx, accountName, account); err != nil {
		loadbalancerlog.Info("failed to fetch owning service group", "error", err)
		return
	}

	// Add a service id to this service
	serviceID, err := allocateServiceID(ctx, crtclient, account)
	if err != nil {
		loadbalancerlog.Info("failed to allocate serviceID", "error", err)
	}
	r.Spec.ServiceID = serviceID

	// If the user hasn't provided an Envoy config template, copy it
	// from the owning LBServiceGroup
	if r.Spec.EnvoyTemplate == (*marin3r.EnvoyConfigSpec)(nil) {
		r.Spec.EnvoyTemplate = &sg.Spec.EnvoyTemplate
	}

	// Generate the random tunnel key that we'll use to authenticate the
	// EGO in the GUE tunnel
	if r.Spec.TunnelKey == "" {
		r.Spec.TunnelKey = generateTunnelKey()
	}

	// Add an external address if needed
	if r.Spec.PublicAddress == "" {
		address, err := allocator.AllocateFromPool(r.NamespacedName().String(), poolName, r.Spec.PublicPorts, "")
		if err != nil {
			loadbalancerlog.Error(err, "allocation failed")
			return
		}
		r.Spec.PublicAddress = address.String()
	}

	loadbalancerlog.Info("defaulted", "name", r.Name, "contents", r)
}

// +kubebuilder:webhook:verbs=create;delete,path=/validate-epic-acnodal-io-v1-loadbalancer,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=loadbalancers,versions=v1,name=vloadbalancer.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Validator = &LoadBalancer{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateCreate() error {
	loadbalancerlog.Info("validate create", "name", r.Name, "contents", r)

	// Block create if there's no owning SG
	if _, err := r.fetchLBServiceGroup(); err != nil {
		return err
	}

	// Block create if the IP allocation failed
	if r.Spec.PublicAddress == "" {
		return fmt.Errorf("%s has no public address", r.Name)
	}

	return nil
}

// ValidateUpdate does nothing.
func (r *LoadBalancer) ValidateUpdate(old runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LoadBalancer) ValidateDelete() error {
	loadbalancerlog.Info("validate delete", "name", r.Name)

	// Don't allow deletion if there are any upstream clusters still
	// using this LB
	if len(r.Spec.UpstreamClusters) > 0 {
		return fmt.Errorf("LB %s has upstream clusters, can't delete: %v", r.NamespacedName(), r.Spec.UpstreamClusters)
	}

	return nil
}

func (r *LoadBalancer) fetchLBServiceGroup() (*LBServiceGroup, error) {
	// Block create if there's no owning SG
	if r.Labels[OwningLBServiceGroupLabel] == "" {
		return nil, fmt.Errorf("LB has no owning service group label")
	}
	sgName := types.NamespacedName{Namespace: r.Namespace, Name: r.Labels[OwningLBServiceGroupLabel]}
	sg := &LBServiceGroup{}
	if err := crtclient.Get(context.TODO(), sgName, sg); err != nil {
		replog.Info("bad input: no owning service group", "name", sgName)
		return nil, err
	}

	return sg, nil
}

// allocateServiceID allocates a service id from the Account that owns
// this LB. If this call succeeds (i.e., error is nil) then the
// returned service id will be unique.
func allocateServiceID(ctx context.Context, cl client.Client, acct *Account) (serviceID uint16, err error) {
	tries := 3
	for err = fmt.Errorf(""); err != nil && tries > 0; tries-- {
		serviceID, err = nextServiceID(ctx, cl, acct)
		if err != nil {
			accountlog.Info("problem allocating account serviceID", "error", err)
		}
	}
	return serviceID, err
}

// nextServiceID gets the next LB ServiceID by doing a
// read-modify-write cycle. It might be inefficient in terms of not
// using all of the values that it allocates but it's safe because the
// Update() will only succeed if the LBServiceGroup hasn't been modified
// since the Get().
//
// This function doesn't retry so if there's a collision with some
// other process the caller needs to retry.
func nextServiceID(ctx context.Context, cl client.Client, acct *Account) (serviceID uint16, err error) {

	// increment the SGs service ID counter
	acct.Status.CurrentServiceID++

	return acct.Status.CurrentServiceID, cl.Status().Update(ctx, acct)
}

// generateTunnelKey generates a 128-bit tunnel key and returns it as
// a base64-encoded string.
func generateTunnelKey() string {
	raw := make([]byte, 16, 16)
	_, _ = rand.Read(raw)
	return base64.StdEncoding.EncodeToString(raw)
}
