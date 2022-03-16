package v1

import (
	"context"
	"fmt"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var (
	gwLog       = logf.Log.WithName("gwproxy-resource")
	gwallocator PoolAllocator
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *GWProxy) SetupWebhookWithManager(mgr ctrl.Manager, alloc PoolAllocator) error {
	gwallocator = alloc
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=accounts,verbs=get;list;watch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=accounts/status,verbs=get;update

// +kubebuilder:webhook:verbs=create,path=/mutate-epic-acnodal-io-v1-gwproxy,mutating=true,failurePolicy=fail,groups=epic.acnodal.io,resources=gwproxies,versions=v1,name=vgwproxy.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Defaulter = &GWProxy{}

// Default sets default values for this GWProxy object.
func (r *GWProxy) Default() {
	ctx := context.TODO()
	gwLog.Info("default", "name", r.Name)

	// Add the controller as a finalizer so we can clean up when this
	// GWProxy is deleted.
	controllerutil.AddFinalizer(r, FinalizerName)

	// Fetch this LB's owning service group
	sg, err := r.getLBServiceGroup(ctx, crtclient)
	if err != nil {
		gwLog.Error(err, "failed to fetch owning service group")
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

	// Generate the random tunnel key that we'll use to authenticate the
	// EGO in the GUE tunnel
	if r.Spec.TunnelKey == "" {
		r.Spec.TunnelKey = generateTunnelKey()
	}

	// Add an external address if needed
	if r.Spec.PublicAddress == "" {
		address, err := gwallocator.AllocateFromPool(r.NamespacedName().String(), poolName, r.Spec.PublicPorts, "")
		if err != nil {
			gwLog.Error(err, "allocation failed")
			return
		}
		r.Spec.PublicAddress = address.String()

		// add an Endpoint struct for external-dns
		r.AddDNSEndpoint(*sg)
	}

	gwLog.Info("defaulted", "name", r.Name, "contents", r)
}

// +kubebuilder:webhook:verbs=create;delete,path=/validate-epic-acnodal-io-v1-gwproxy,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=gwproxies,versions=v1,name=vgwproxy.kb.io,sideEffects=None,admissionReviewVersions=v1

var _ webhook.Validator = &GWProxy{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *GWProxy) ValidateCreate() error {
	ctx := context.TODO()
	gwLog.Info("validate create", "name", r.Name, "contents", r)

	// Block create if there's no owning SG
	sg, err := r.getLBServiceGroup(ctx, crtclient)
	if err != nil {
		return err
	}

	// Block create if there's no owning account
	acct, err := sg.getAccount(ctx, crtclient)
	if err != nil {
		return err
	}

	// Block create if the IP allocation failed
	if r.Spec.PublicAddress == "" {
		return fmt.Errorf("%s/%s IP address allocation failed", r.Namespace, r.Name)
	}

	// See how many proxies have already been created in this account.
	proxyListOpts := client.ListOptions{Namespace: r.Namespace}
	proxies := GWProxyList{}
	if err := crtclient.List(ctx, &proxies, &proxyListOpts); err != nil {
		return err
	}

	// If the user has created too many proxies then deny this one.
	if len(proxies.Items) >= acct.Spec.ProxyLimit {
		return fmt.Errorf("Account proxy limit exceeded, please contact Acnodal")
	}

	return nil
}

// ValidateUpdate does nothing.
func (r *GWProxy) ValidateUpdate(old runtime.Object) error {
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *GWProxy) ValidateDelete() error {
	return nil
}
