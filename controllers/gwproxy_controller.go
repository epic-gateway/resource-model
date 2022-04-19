package controllers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
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
)

// GWProxyReconciler reconciles a GWProxy object
type GWProxyReconciler struct {
	client.Client
	Allocator     *allocator.Allocator
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.marin3r.3scale.net,resources=envoydeployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *GWProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("Reconciling")

	prefix := &epicv1.ServicePrefix{}
	account := &epicv1.Account{}
	sg := &epicv1.LBServiceGroup{}
	epic := &epicv1.EPIC{}

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

	// Determine the Envoy Image to run. EPIC singleton is the default,
	// and LBSG overrides.
	envoyImage := epic.Spec.EnvoyImage
	if sg.Spec.EnvoyImage != nil {
		envoyImage = *sg.Spec.EnvoyImage
	}

	// Launch the Envoy deployment that will implement this proxy.
	dep := r.deploymentForProxy(proxy, prefix, envoyImage, epic.Spec.ServiceCIDR)
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

	// Allocate tunnels
	if err := allocateProxyTunnels(ctx, r.Client, l, proxy); err != nil {
		return done, err
	}

	// Find any Routes that reference this Proxy.
	inputRoutes, err := proxy.GetChildRoutes(ctx, r.Client, l)
	if err != nil {
		return done, err
	}
	l.V(1).Info("Input routes", "count", len(inputRoutes), "routes", inputRoutes)

	// Pre-process Routes before templating them.
	templateRoutes, err := envoy.PreprocessRoutes(inputRoutes)
	if err != nil {
		return done, err
	}
	l.V(1).Info("Preprocessed routes", "count", len(templateRoutes), "routes", templateRoutes)

	// Build a new EnvoyConfig
	envoyConfig, err := envoy.GWProxyToEnvoyConfig(*proxy, templateRoutes)
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
func (r *GWProxyReconciler) deploymentForProxy(proxy *epicv1.GWProxy, sp *epicv1.ServicePrefix, envoyImage string, serviceCIDR string) *marin3roperator.EnvoyDeployment {
	// Format the pod's IP address by parsing the raw address and adding
	// the netmask from the service prefix Subnet
	addr, err := netlink.ParseAddr(proxy.Spec.PublicAddress + "/32")
	if err != nil {
		return nil
	}
	subnet, err := sp.Spec.PublicPool.SubnetIPNet()
	if err != nil {
		return nil
	}
	addr.Mask = subnet.Mask
	addresses := []string{addr.String()}

	if proxy.Spec.AltAddress != "" {
		addrAlt, err := netlink.ParseAddr(proxy.Spec.AltAddress + "/32")
		if err != nil {
			return nil
		}
		subnetAlt, err := sp.Spec.AltPool.SubnetIPNet()
		if err == nil {
			addrAlt.Mask = subnetAlt.Mask
			addresses = append(addresses, addrAlt.String())
		}
	}

	// Format the multus configuration that tells multus which IP
	// address to attach to the pod's net1 interface.
	multusConfig, err := json.Marshal([]map[string]interface{}{{
		"default-route": []string{},
		"name":          sp.Name,
		"namespace":     sp.Namespace,
		"ips":           addresses,
	}})
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

// allocateProxyTunnels adds a tunnel from each endpoints item to each
// node running an Envoy proxy and patches proxy with the tunnel info.
func allocateProxyTunnels(ctx context.Context, cl client.Client, l logr.Logger, proxy *epicv1.GWProxy) error {
	var (
		err        error
		newMap     map[string]epicv1.EPICEndpointMap = map[string]epicv1.EPICEndpointMap{}
		patch      []map[string]interface{}
		patchBytes []byte
		raw        []byte = make([]byte, 8, 8)
	)

	_, _ = rand.Read(raw)

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &epicv1.EPIC{}
	if err := cl.Get(ctx, types.NamespacedName{Name: epicv1.ConfigName, Namespace: epicv1.ConfigNamespace}, config); err != nil {
		return err
	}

	// Get all of the client-side endpoints for this proxy
	endpoints, err := proxy.ActiveProxyEndpoints(ctx, cl)
	if err != nil {
		return err
	}

	// We set up a tunnel from each proxy to each of this slice's nodes.
	for _, clientPod := range endpoints {
		for _, proxyPod := range proxy.Spec.ProxyInterfaces {

			// If we haven't seen this client node yet then initialize its
			// map.
			if _, seenBefore := newMap[clientPod.Spec.NodeAddress]; !seenBefore {
				newMap[clientPod.Spec.NodeAddress] = epicv1.EPICEndpointMap{
					EPICEndpoints: map[string]epicv1.GUETunnelEndpoint{},
				}
			}

			if tunnel, exists := proxy.Spec.GUETunnelMaps[clientPod.Spec.NodeAddress].EPICEndpoints[proxyPod.EPICNodeAddress]; exists {
				// If the tunnel already exists (i.e., two client endpoints in
				// the same service on the same node) then we can just copy
				// the map from the old resource into the patch.
				newMap[clientPod.Spec.NodeAddress].EPICEndpoints[proxyPod.EPICNodeAddress] = tunnel
			} else {
				// This is a new proxyNode/endpointNode pair, so allocate a new
				// tunnel ID for it.
				envoyEndpoint := epicv1.GUETunnelEndpoint{}
				config.Spec.NodeBase.GUEEndpoint.DeepCopyInto(&envoyEndpoint)
				envoyEndpoint.Address = proxyPod.EPICNodeAddress
				if envoyEndpoint.TunnelID, err = allocateTunnelID(ctx, l, cl); err != nil {
					return err
				}
				newMap[clientPod.Spec.NodeAddress].EPICEndpoints[proxyPod.EPICNodeAddress] = envoyEndpoint
			}
		}
	}

	// Prepare the patch.

	// If there's already a map then delete it.
	if len(proxy.Spec.GUETunnelMaps) > 0 {
		patch = append(
			patch,
			map[string]interface{}{
				"op":   "remove",
				"path": "/spec/gue-tunnel-endpoints",
			},
		)
	}
	// Add the new tunnel map.
	patch = append(
		patch,
		map[string]interface{}{
			"op":    "add",
			"path":  "/spec/gue-tunnel-endpoints",
			"value": newMap,
		},
	)

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	l.Info(string(patchBytes))
	if err := cl.Patch(ctx, proxy, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		l.Error(err, "patching", "proxy", proxy)
		return err
	}
	l.Info("patched", "proxy", proxy)

	return nil
}
