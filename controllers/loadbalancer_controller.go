package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	marin3roperator "github.com/3scale-ops/marin3r/apis/operator.marin3r/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
)

// LoadBalancerReconciler reconciles a LoadBalancer object
type LoadBalancerReconciler struct {
	client.Client
	Log           logr.Logger
	Allocator     *allocator.Allocator
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.marin3r.3scale.net,resources=envoydeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crd.projectcalico.org,resources=ippools,verbs=get;list

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *LoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("loadbalancer", req.NamespacedName)
	log.Info("reconciling")

	prefix := &epicv1.ServicePrefix{}
	account := &epicv1.Account{}
	sg := &epicv1.LBServiceGroup{}
	epic := &epicv1.EPIC{}

	// read the object that caused the event
	lb := &epicv1.LoadBalancer{}
	if err := r.Get(ctx, req.NamespacedName, lb); err != nil {
		log.Info("can't get resource, probably deleted", "loadbalancer", req.NamespacedName)
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Now that we know the resource version we can use it in our log
	// messages
	log = r.Log.WithValues("loadbalancer", req.NamespacedName, "version", lb.ResourceVersion)

	// Check if k8s wants to delete this object
	if !lb.ObjectMeta.DeletionTimestamp.IsZero() {

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
		if err := RemoveFinalizer(ctx, r.Client, lb, epicv1.LoadbalancerFinalizerName); err != nil {
			return done, err
		}

		// Stop reconciliation as the item is being deleted
		return done, nil
	}

	// Get the "stack" of CRs to which this LB belongs: LBServiceGroup,
	// ServicePrefix, Account, and EPIC singleton. They provide
	// configuration data that we need but that isn't contained in the
	// LB.
	sgName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Labels[epicv1.OwningLBServiceGroupLabel]}
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

	// Determine the Envoy Image to run. EPIC singleton is the default,
	// and LBSG overrides.
	envoyImage := epic.Spec.EnvoyImage
	if sg.Spec.EnvoyImage != nil {
		envoyImage = *sg.Spec.EnvoyImage
	}

	// Launch the Envoy deployment that will proxy this LB
	dep := r.deploymentForLB(lb, prefix, envoyImage, epic.Spec.ServiceCIDR)
	if err := r.Create(ctx, dep); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Info("Failed to create new Deployment", "message", err.Error(), "namespace", dep.Namespace, "name", dep.Name)
			return done, err
		}
		log.Info("deployment created previously, will update", "namespace", dep.Namespace, "name", dep.Name, "replicas", dep.Spec.Replicas)
		if err := updateDeployment(ctx, r.Client, dep); err != nil {
			log.Info("Failed to update Deployment", "message", err.Error(), "namespace", dep.Namespace, "name", dep.Name)
			return done, err
		}
	} else {
		log.Info("deployment created", "namespace", dep.Namespace, "name", dep.Name)
	}

	// List the endpoints that belong to this LB in case this is an
	// update to an LB that already has endpoints. The remoteendpoints
	// controller will set up the PFC stuff but we need to make sure
	// that all of the endpoints are accounted for in the Envoy config.
	reps, err := listActiveLBEndpoints(ctx, r, lb)
	if err != nil {
		return done, err
	}
	log.Info("endpoints", "endpoints", reps)

	// Build a new EnvoyConfig
	envoyConfig, err := envoy.ServiceToEnvoyConfig(*lb, reps)
	if err != nil {
		return done, err
	}
	ctrl.SetControllerReference(lb, &envoyConfig, r.Scheme())

	// Create/update the marin3r EnvoyConfig. We'll Create() first since
	// that's probably the most common case. If Create() fails then it
	// might be an update, e.g., someone tweaking their Envoy config
	// template.
	if err := r.Create(ctx, &envoyConfig); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Info("Failed to create new EnvoyConfig", "message", err.Error(), "namespace", envoyConfig.Namespace, "name", envoyConfig.Name)
			return done, err
		}
		log.Info("existing EnvoyConfig, will update", "namespace", envoyConfig.Namespace, "name", envoyConfig.Name)
		existing := marin3r.EnvoyConfig{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: lb.Namespace, Name: lb.Name}, &existing); err != nil {
			return done, err
		}
		envoyConfig.Spec.DeepCopyInto(&existing.Spec)
		return done, r.Update(ctx, &existing)
	}

	log.Info("EnvoyConfig created", "namespace", envoyConfig.Namespace, "name", envoyConfig.Name)
	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *LoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.LoadBalancer{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *LoadBalancerReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

func envoyPodName(lb *epicv1.LoadBalancer) string {
	// this should let us see that:
	//  - it's an envoy
	//  - it's working for the lb named lb.Name
	return lb.Name
}

// deploymentForLB returns an EnvoyDeployment object that will tell
// Marin3r to launch Envoy pods to serve lb.
func (r *LoadBalancerReconciler) deploymentForLB(lb *epicv1.LoadBalancer, sp *epicv1.ServicePrefix, envoyImage string, serviceCIDR string) *marin3roperator.EnvoyDeployment {
	// Format the pod's IP address by parsing the raw address and adding
	// the netmask from the service prefix Subnet
	addr, err := netlink.ParseAddr(lb.Spec.PublicAddress + "/32")
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

	// this should let us see that:
	//  - it's an envoy
	//  - it's working for the lb named lb.Name
	name := envoyPodName(lb)

	dep := &marin3roperator.EnvoyDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lb.Namespace,
		},
		Spec: marin3roperator.EnvoyDeploymentSpec{
			DiscoveryServiceRef: epicv1.DiscoveryServiceName,
			EnvoyConfigRef:      name,
			Image:               pointer.StringPtr(envoyImage),
			ExtraLabels:         epicv1.LabelsForEnvoy(lb.Name),
			Replicas: &marin3roperator.ReplicasSpec{
				Static: lb.Spec.EnvoyReplicaCount,
			},
			Ports: portsToPorts(lb.Spec.PublicPorts),
			Annotations: map[string]string{
				cniAnnotation: string(multusConfig),
			},
			EnvVars: []corev1.EnvVar{{
				Name:  serviceCIDREnv,
				Value: serviceCIDR,
			}},
		},
	}

	// If this is *not* a TrueIngress LB then set the env var that tells
	// our Envoy docker-entrypoint to *not* set it up.
	if !lb.Spec.TrueIngress {
		dep.Spec.EnvVars = []corev1.EnvVar{{
			Name:  "TRUEINGRESS",
			Value: "disabled",
		}}
	}

	// Set LB instance as the owner and controller
	ctrl.SetControllerReference(lb, dep, r.Scheme())
	return dep
}

func splitNSName(name string) (*types.NamespacedName, error) {
	parts := strings.Split(name, string(types.Separator))
	if len(parts) < 2 {
		return nil, fmt.Errorf("Malformed NamespaceName: %q", parts)
	}

	return &types.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
}

// portsToPorts converts from ServicePorts to ContainerPorts.
func portsToPorts(sPorts []corev1.ServicePort) []marin3roperator.ContainerPort {
	cPorts := make([]marin3roperator.ContainerPort, len(sPorts))

	// Expose the configured ports
	for i, port := range sPorts {
		proto := washProtocol(port.Protocol)
		cPorts[i] = marin3roperator.ContainerPort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: &proto,
		}
	}

	return cPorts
}

// washProtocol "washes" proto, optionally upcasing if necessary.
func washProtocol(proto corev1.Protocol) corev1.Protocol {
	return corev1.Protocol(strings.ToUpper(string(proto)))
}

// listActiveLBEndpoints lists the endpoints that belong to lb that
// are active, i.e., not in the process of being deleted.
func listActiveLBEndpoints(ctx context.Context, cl client.Client, lb *epicv1.LoadBalancer) ([]epicv1.RemoteEndpoint, error) {
	labelSelector := labels.SelectorFromSet(map[string]string{epicv1.OwningLoadBalancerLabel: lb.Name})
	listOps := client.ListOptions{Namespace: lb.Namespace, LabelSelector: labelSelector}
	list := epicv1.RemoteEndpointList{}
	err := cl.List(ctx, &list, &listOps)

	activeEPs := []epicv1.RemoteEndpoint{}
	// build a new list with no "in deletion" endpoints
	for _, endpoint := range list.Items {
		if endpoint.ObjectMeta.DeletionTimestamp.IsZero() {
			activeEPs = append(activeEPs, endpoint)
		}
	}

	return activeEPs, err
}

func updateDeployment(ctx context.Context, cl client.Client, updated *marin3roperator.EnvoyDeployment) error {
	key := client.ObjectKey{Namespace: updated.GetNamespace(), Name: updated.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := updated.DeepCopy()

		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, existing); err != nil {
			return err
		}
		updated.Spec.DeepCopyInto(&existing.Spec)

		// Try to update
		return cl.Update(ctx, existing)
	})
}
