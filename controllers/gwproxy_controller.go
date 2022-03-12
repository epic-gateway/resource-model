package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	marin3roperator "github.com/3scale-ops/marin3r/apis/operator.marin3r/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/allocator"
	"gitlab.com/acnodal/epic/resource-model/internal/envoy"
)

const (
	gitlabSecret  = "gitlab"
	cniAnnotation = "k8s.v1.cni.cncf.io/networks"

	// serviceCIDREnv is the name of the env var that tells the Envoy
	// pod the CIDR that contains internal service addresses. The Envoy
	// image uses this to set up routing between its two interfaces.
	serviceCIDREnv = "SERVICE_CIDR"
	// podCIDREnv is the name of the env var that tells the Envoy pod
	// the CIDR that contains internal pod addresses. The Envoy image
	// uses this to set up routing between its two interfaces.
	podCIDREnv = "POD_CIDR"
)

// GWProxyReconciler reconciles a GWProxy object
type GWProxyReconciler struct {
	client.Client
	Log           logr.Logger
	Allocator     *allocator.Allocator
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.marin3r.3scale.net,resources=envoydeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *GWProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.Info("reconciling")

	prefix := &epicv1.ServicePrefix{}
	account := &epicv1.Account{}
	sg := &epicv1.LBServiceGroup{}
	epic := &epicv1.EPIC{}
	node := &corev1.Node{}

	// read the object that caused the event
	proxy := &epicv1.GWProxy{}
	if err := r.Get(ctx, req.NamespacedName, proxy); err != nil {
		l.Info("can't get resource, probably deleted", "gwproxy", req.NamespacedName)
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Now that we know the resource version we can use it in our log
	// messages
	l = l.WithValues("version", proxy.ResourceVersion)

	// Check if k8s wants to delete this object
	if !proxy.ObjectMeta.DeletionTimestamp.IsZero() {

		// From here until we return at the end of this "if", everything
		// is "best-effort", i.e., we try but if something fails we keep
		// going to ensure that we try everything

		// Return the LB's address to its pool
		if !r.Allocator.Unassign(req.NamespacedName.String()) {
			fmt.Printf("ERROR freeing address from %s\n", req.NamespacedName.String())
			// Continue - we want to delete the CR even if something went
			// wrong with this Unassign
		}

		// Remove our finalizer from the list and update the lb. If
		// anything goes wrong below we don't want to block the deletion
		// of the CR
		if err := RemoveFinalizer(ctx, r.Client, proxy, epicv1.FinalizerName); err != nil {
			return done, err
		}

		// Stop reconciliation as the item is being deleted
		return done, nil
	}

	// Get the "stack" of CRs to which this LB belongs: LBServiceGroup,
	// ServicePrefix, Account, and EPIC singleton. They provide
	// configuration data that we need but that isn't contained in the
	// LB.
	sgName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: proxy.Labels[epicv1.OwningLBServiceGroupLabel]}
	if err := r.Get(ctx, sgName, sg); err != nil {
		return done, err
	}

	prefixName := types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: sg.Labels[epicv1.OwningServicePrefixLabel]}
	if err := r.Get(ctx, prefixName, prefix); err != nil {
		return done, err
	}

	accountName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: sg.Labels[epicv1.OwningAccountLabel]}
	if err := r.Get(ctx, accountName, account); err != nil {
		return done, err
	}

	epicName := types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: epicv1.ConfigName}
	if err := r.Get(ctx, epicName, epic); err != nil {
		return done, err
	}

	nodeName := types.NamespacedName{Namespace: "", Name: os.Getenv("EPIC_NODE_NAME")}
	if err := r.Get(ctx, nodeName, node); err != nil {
		return done, err
	}

	// Determine the Envoy Image to run. EPIC singleton is the default,
	// and LBSG overrides.
	envoyImage := epic.Spec.EnvoyImage
	if sg.Spec.EnvoyImage != nil {
		envoyImage = *sg.Spec.EnvoyImage
	}

	// Launch the Envoy deployment that will implement this proxy.
	dep := r.deploymentForProxy(proxy, prefix, envoyImage, epic.Spec.ServiceCIDR, node.Spec.PodCIDR)
	if err := r.Create(ctx, dep); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			l.Info("Failed to create new Deployment", "message", err.Error(), "namespace", dep.Namespace, "name", dep.Name)
			return done, err
		}
		l.Info("deployment created previously, will update", "namespace", dep.Namespace, "name", dep.Name, "replicas", dep.Spec.Replicas)
		if err := updateDeployment(ctx, r.Client, dep); err != nil {
			l.Info("Failed to update Deployment", "message", err.Error(), "namespace", dep.Namespace, "name", dep.Name)
			return done, err
		}
	} else {
		l.Info("deployment created", "namespace", dep.Namespace, "name", dep.Name)
	}

	// Find any Routes that point to this Proxy
	routes, err := proxy.GetChildRoutes(ctx, r.Client, l)
	if err != nil {
		return done, err
	}
	l.Info("Routes for Proxy", "count", len(routes), "routes", routes)

	// Build a new EnvoyConfig
	envoyConfig, err := envoy.GWProxyToEnvoyConfig(*proxy, routes)
	if err != nil {
		return done, err
	}
	ctrl.SetControllerReference(proxy, &envoyConfig, r.Scheme())

	// Create/update the marin3r EnvoyConfig. We'll Create() first since
	// that's probably the most common case. If Create() fails then it
	// might be an update, e.g., someone tweaking their Envoy config
	// template.
	if err := r.Create(ctx, &envoyConfig); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			l.Error(err, "Failed to create new EnvoyConfig", "namespace", envoyConfig.Namespace, "name", envoyConfig.Name, "config", envoyConfig)
			return done, err
		}
		l.Info("existing EnvoyConfig, will update", "namespace", envoyConfig.Namespace, "name", envoyConfig.Name)
		existing := marin3r.EnvoyConfig{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: proxy.Namespace, Name: proxy.Name}, &existing); err != nil {
			return done, err
		}
		envoyConfig.Spec.DeepCopyInto(&existing.Spec)
		return done, r.Update(ctx, &existing)
	}

	l.Info("EnvoyConfig created", "namespace", envoyConfig.Namespace, "name", envoyConfig.Name)
	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *GWProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWProxy{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *GWProxyReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// deploymentForProxy returns an EnvoyDeployment object that will tell
// Marin3r to launch Envoy pods to serve for this proxy.
func (r *GWProxyReconciler) deploymentForProxy(proxy *epicv1.GWProxy, sp *epicv1.ServicePrefix, envoyImage string, serviceCIDR string, podCIDR string) *marin3roperator.EnvoyDeployment {
	// Format the pod's IP address by parsing the raw address and adding
	// the netmask from the service prefix Subnet
	addr, err := netlink.ParseAddr(proxy.Spec.PublicAddress + "/32")
	if err != nil {
		return nil
	}
	subnet, err := sp.Spec.SubnetIPNet()
	if err != nil {
		return nil
	}
	addr.Mask = subnet.Mask

	// Format the multus configuration that tells multus which IP
	// address to attach to the pod's net1 interface.
	multusConfig, err := json.Marshal([]map[string]interface{}{{
		"name":      sp.Name,
		"namespace": sp.Namespace,
		"ips":       []string{addr.String()}},
	})
	if err != nil {
		return nil
	}

	name := proxy.Name

	dep := &marin3roperator.EnvoyDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: proxy.Namespace,
		},
		Spec: marin3roperator.EnvoyDeploymentSpec{
			DiscoveryServiceRef: epicv1.DiscoveryServiceName,
			EnvoyConfigRef:      name,
			Image:               pointer.StringPtr(envoyImage),
			ExtraLabels:         epicv1.LabelsForProxy(proxy.Name),
			Replicas: &marin3roperator.ReplicasSpec{
				Static: proxy.Spec.EnvoyReplicaCount,
			},
			Ports: portsToPorts(proxy.Spec.PublicPorts),
			Annotations: map[string]string{
				cniAnnotation: string(multusConfig),
			},
			EnvVars: []corev1.EnvVar{
				{
					Name:  serviceCIDREnv,
					Value: serviceCIDR,
				},
				{
					Name:  podCIDREnv,
					Value: podCIDR,
				},
				{
					Name: "HOST_IP",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "status.hostIP",
						},
					},
				},
			},
		},
	}

	// Set GWProxy instance as the owner and controller
	ctrl.SetControllerReference(proxy, dep, r.Scheme())
	return dep
}
