package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vishvananda/netlink"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/allocator"
)

// LoadBalancerReconciler reconciles a LoadBalancer object
type LoadBalancerReconciler struct {
	client.Client
	Allocator     *allocator.Allocator
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.marin3r.3scale.net,resources=envoydeployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *LoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("reconciling")

	prefix := &epicv1.ServicePrefix{}
	account := &epicv1.Account{}
	sg := &epicv1.LBServiceGroup{}
	epic := &epicv1.EPIC{}

	// read the object that caused the event
	lb := &epicv1.LoadBalancer{}
	if err := r.Get(ctx, req.NamespacedName, lb); err != nil {
		l.Info("can't get resource, probably deleted", "loadbalancer", req.NamespacedName)
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// Now that we know the resource version we can use it in our log
	// messages
	l = l.WithValues("resourceVersion", lb.ResourceVersion)

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
		if err := RemoveFinalizer(ctx, r.Client, lb, epicv1.FinalizerName); err != nil {
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

// deploymentForLB returns a Deployment object to proxy this LB.
func (r *LoadBalancerReconciler) deploymentForLB(lb *epicv1.LoadBalancer, sp *epicv1.ServicePrefix, envoyImage string, serviceCIDR string) *appsv1.Deployment {
	// Format the pod's IP address by parsing the raw address and adding
	// the netmask from the service prefix Subnet
	addr, err := netlink.ParseAddr(lb.Spec.PublicAddress + "/32")
	if err != nil {
		return nil
	}
	subnet, err := sp.Spec.PublicPool.SubnetIPNet()
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
	labels := epicv1.LabelsForEnvoy(lb.Name)
	labels["app.kubernetes.io/component"] = "envoy-deployment"

	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lb.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						cniAnnotation: string(multusConfig),
					},
				},
				Spec: v1.PodSpec{
					Hostname:  epicv1.EDSServerName,
					Subdomain: "epic",
					ImagePullSecrets: []v1.LocalObjectReference{
						{Name: "gitlab"},
					},
					Containers: []v1.Container{{
						Name:            epicv1.EDSServerName,
						Image:           envoyImage,
						ImagePullPolicy: v1.PullAlways,
						Env: []v1.EnvVar{
							{
								Name: "WATCH_NAMESPACE",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										FieldPath: "metadata.namespace",
									},
								},
							},
							{
								Name:  serviceCIDREnv,
								Value: serviceCIDR,
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
						Ports: portsToV1Ports(lb.Spec.PublicPorts),
						SecurityContext: &v1.SecurityContext{
							Capabilities: &v1.Capabilities{
								Add: []v1.Capability{"NET_ADMIN"},
							},
							AllowPrivilegeEscalation: pointer.BoolPtr(true),
							ReadOnlyRootFilesystem:   pointer.BoolPtr(false),
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "server-cert",
								MountPath: "/etc/envoy/tls/server/",
								ReadOnly:  true,
							},
							{
								Name:      "ca-cert",
								MountPath: "/etc/envoy/tls/ca/",
								ReadOnly:  true,
							},
						},
					}},
					// ServiceAccountName:            epicv1.EDSServerName,
					TerminationGracePeriodSeconds: pointer.Int64Ptr(3),
					Volumes: []v1.Volume{
						{
							Name: "server-cert",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									DefaultMode: pointer.Int32Ptr(420),
									SecretName:  "marin3r-server-cert-discoveryservice",
								},
							},
						},
						{
							Name: "ca-cert",
							VolumeSource: v1.VolumeSource{
								Secret: &v1.SecretVolumeSource{
									DefaultMode: pointer.Int32Ptr(420),
									SecretName:  "marin3r-ca-cert-discoveryservice",
								},
							},
						},
					},
				},
			},
		},
	}

	// Set LB instance as the owner and controller
	ctrl.SetControllerReference(lb, &dep, r.Scheme())
	return &dep
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

// portsToPorts converts from ServicePorts to ContainerPorts.
func portsToV1Ports(sPorts []corev1.ServicePort) []v1.ContainerPort {
	cPorts := make([]v1.ContainerPort, len(sPorts))

	// Expose the configured ports
	for i, port := range sPorts {
		proto := washProtocol(port.Protocol)
		cPorts[i] = v1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.Port,
			Protocol:      proto,
		}
	}

	return cPorts
}
