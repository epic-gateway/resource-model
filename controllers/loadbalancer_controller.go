package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	"github.com/vishvananda/netlink"
	appsv1 "k8s.io/api/apps/v1"
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

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *LoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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

	log := r.Log.WithValues("loadbalancer", req.NamespacedName, "version", lb.ResourceVersion)
	log.Info("reconciling")

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
	dep := r.deploymentForLB(lb, prefix, envoyImage)
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
	return "envoy-" + lb.Name
}

// deploymentForLB returns a Deployment object that will launch an
// Envoy pod
func (r *LoadBalancerReconciler) deploymentForLB(lb *epicv1.LoadBalancer, sp *epicv1.ServicePrefix, envoyImage string) *appsv1.Deployment {
	labels := epicv1.LabelsForEnvoy(lb.Name)

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

	// format a fragment of Envoy config yaml that will set the dynamic
	// command-line overrides that differentiate this instance of Envoy
	// from the others.
	envoyOverrides := fmt.Sprintf("node: {cluster: xds_cluster, id: %s.%s}", lb.Namespace, lb.Name)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lb.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: lb.Spec.EnvoyReplicaCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{cniAnnotation: string(multusConfig)},
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: pointer.Int64Ptr(0),
					Containers: []corev1.Container{
						{Image: envoyImage,
							ImagePullPolicy: corev1.PullAlways,
							Name:            name,
							Ports:           portsToPorts(lb.Spec.PublicPorts),
							Command:         []string{"/docker-entrypoint.sh"},
							Args:            []string{"envoy", "--config-path", "/etc/envoy/envoy.yaml", "--config-yaml", envoyOverrides, "--log-level", "debug"},
							SecurityContext: &corev1.SecurityContext{
								Privileged:               pointer.BoolPtr(true),
								AllowPrivilegeEscalation: pointer.BoolPtr(true),
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"NET_ADMIN"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "envoy-sidecar-tls",
									MountPath: "/etc/envoy/tls/client",
									ReadOnly:  true,
								},
								{
									Name:      "envoy-sidecar-bootstrap-v3",
									MountPath: "/etc/envoy/bootstrap",
									ReadOnly:  true,
								},
							},
						},
					},
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
						{
							// Spread pods across nodes as much as possible
							TopologyKey:       "kubernetes.io/hostname",
							MaxSkew:           1,
							WhenUnsatisfiable: "DoNotSchedule",
						},
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: gitlabSecret},
					},
					Volumes: []corev1.Volume{
						{
							Name: "envoy-sidecar-tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  "envoy-sidecar-client-cert",
									DefaultMode: pointer.Int32Ptr(420),
								},
							},
						},
						{
							Name: "envoy-sidecar-bootstrap-v3",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "envoy-sidecar-bootstrap-v3",
									},
									DefaultMode: pointer.Int32Ptr(420),
								},
							},
						},
					},
				},
			},
		},
	}

	// If this is *not* a TrueIngress LB then set the env var that tells
	// our Envoy docker-entrypoint to *not* set it up.
	if !lb.Spec.TrueIngress {
		dep.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{
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
func portsToPorts(sPorts []corev1.ServicePort) []corev1.ContainerPort {
	cPorts := make([]corev1.ContainerPort, len(sPorts))
	for i, port := range sPorts {
		cPorts[i] = corev1.ContainerPort{Protocol: washProtocol(port.Protocol), ContainerPort: port.Port}
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

func updateDeployment(ctx context.Context, cl client.Client, updated *appsv1.Deployment) error {
	key := client.ObjectKey{Namespace: updated.GetNamespace(), Name: updated.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := appsv1.Deployment{}

		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, &existing); err != nil {
			return err
		}
		updated.Spec.DeepCopyInto(&existing.Spec)

		// Try to update
		return cl.Update(ctx, &existing)
	})
}
