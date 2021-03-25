package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	egwv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/allocator"
	"gitlab.com/acnodal/epic/resource-model/internal/envoy"
	egwexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
	"gitlab.com/acnodal/epic/resource-model/internal/pfc"
)

const (
	gitlabSecret  = "gitlab"
	cniAnnotation = "k8s.v1.cni.cncf.io/networks"
)

// LoadBalancerReconciler reconciles a LoadBalancer object
type LoadBalancerReconciler struct {
	client.Client
	Log       logr.Logger
	allocator *allocator.Allocator
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=remoteendpoints,verbs=get;list;delete;deletecollection

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=marin3r.3scale.net,resources=envoyconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *LoadBalancerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	done := ctrl.Result{Requeue: false}
	tryAgain := ctrl.Result{RequeueAfter: 10 * time.Second}
	ctx := context.Background()
	log := r.Log.WithValues("loadbalancer", req.NamespacedName)

	log.Info("reconciling")

	// read the object that caused the event
	lb := &egwv1.LoadBalancer{}
	if err := r.Get(ctx, req.NamespacedName, lb); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// read the ServiceGroup that owns this LB
	sg := &egwv1.ServiceGroup{}
	sgname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Labels[egwv1.OwningServiceGroupLabel]}
	err = r.Get(ctx, sgname, sg)
	if err != nil {
		log.Error(err, "Failed to find owning service group", "name", sgname)
		return done, err
	}

	// read the ServicePrefix that determines the interface that we'll use
	prefixName := types.NamespacedName{Namespace: egwv1.ConfigNamespace, Name: sg.Labels[egwv1.OwningServicePrefixLabel]}
	prefix := &egwv1.ServicePrefix{}
	err = r.Get(ctx, prefixName, prefix)
	if err != nil {
		log.Error(err, "Failed to find owning service prefix", "name", prefixName)
		return done, err
	}

	// get the Account that owns this LB
	accountName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: sg.Labels[egwv1.OwningAccountLabel]}
	account := &egwv1.Account{}
	if err := r.Get(ctx, accountName, account); err != nil {
		return done, err
	}

	// Check if k8s wants to delete this object
	if !lb.ObjectMeta.DeletionTimestamp.IsZero() {

		// The object is being deleted
		if controllerutil.ContainsFinalizer(lb, egwv1.LoadbalancerFinalizerName) {
			log.Info("to be deleted")

			// First, remove our finalizer from the list and update the
			// lb. If anything goes wrong below we don't want to block the
			// deletion of the CR
			controllerutil.RemoveFinalizer(lb, egwv1.LoadbalancerFinalizerName)
			if err := r.Update(ctx, lb); err != nil {
				return done, err
			}

			// From here until we return at the end of this "if", everything
			// is "best-effort", i.e., we try but if something fails we keep
			// going to ensure that we try everything

			// Return the LB's address to its pool
			if !r.allocator.Unassign(req.NamespacedName.String()) {
				fmt.Printf("ERROR freeing address from %s\n", req.NamespacedName.String())
				// Continue - we want to delete the CR even if something went
				// wrong with this Unassign
			}

			// Close host ports by updating IPSET tables
			if err := delIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts); err != nil {
				log.Error(err, "deleting ipset entry")
			}

			// Remove route to public address
			delRt(lb.Spec.PublicAddress)

			// Remove PFC configuration
			if err := r.cleanupPFC(log, lb, account, prefix); err != nil {
				log.Error(err, "Failed to cleanup PFC")
			}

			// Delete the reps that belong to this LB
			opts := []client.DeleteAllOfOption{
				client.InNamespace(req.NamespacedName.Namespace),
				client.MatchingLabels{egwv1.OwningLoadBalancerLabel: req.NamespacedName.Name},
			}
			if err := r.DeleteAllOf(ctx, &egwv1.RemoteEndpoint{}, opts...); err != nil {
				log.Error(err, "Failed to delete endpoints")
			}

			// Delete the service's EnvoyConfig
			ec := marin3r.EnvoyConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: req.NamespacedName.Namespace,
					Name:      req.NamespacedName.Name,
				},
			}
			if err := r.Delete(ctx, &ec); err != nil {
				log.Error(err, "Failed to delete EnvoyConfig")
			}
		}

		// Stop reconciliation as the item is being deleted
		return done, nil
	}

	// Launch the Envoy deployment that will proxy this LB
	dep := r.deploymentForLB(lb, &prefixName, sg.Spec.EnvoyImage)
	err = r.Create(ctx, dep)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			log.Info("Failed to create new Deployment", "message", err.Error(), "namespace", dep.Namespace, "name", dep.Name)
			return done, err
		}
		log.Info("deployment created previously", "namespace", dep.Namespace, "name", dep.Name)
	} else {
		log.Info("deployment created", "namespace", dep.Namespace, "name", dep.Name)
	}

	// We need this LB's veth info so if the python daemon hasn't filled
	// it in yet then we'll back off and give it more time
	if lb.Status.ProxyIfindex == 0 || lb.Status.ProxyIfname == "" {
		log.Info("status has no ifindex")
		return tryAgain, nil
	}
	log.Info("LB status veth info", "ifindex", lb.Status.ProxyIfindex, "ifname", lb.Status.ProxyIfname)

	// add the packet tagger to the Envoy pod veth
	err = r.configureTagging(log, lb.Status.ProxyIfname)
	if err != nil {
		log.Error(err, "adding packet tagger", "if", lb.Status.ProxyIfname)
		return done, err
	}

	// configure the Multus bridge interface
	err = r.configureBridge(prefix.Spec.MultusBridge, prefix.Spec.GatewayAddr())
	if err != nil {
		log.Error(err, "Failed to configure multus bridge", "name", prefixName)
		return done, err
	}

	// Setup host routing
	_, publicaddr, _ := net.ParseCIDR(prefix.Spec.Subnet)
	if prefix.Spec.Aggregation != "default" {
		_, publicaddr, _ = net.ParseCIDR(lb.Spec.PublicAddress + prefix.Spec.Aggregation)
	}

	err = addRt(publicaddr, prefix.Spec.MultusBridge)
	if err != nil {
		log.Error(err, "adding route to multus interface")
	}

	//Open host ports by updating IPSET tables
	if err := addIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts); err != nil {
		log.Error(err, "adding ipset entry")
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

	// Create/update the marin3r EnvoyConfig. We'll Create() first since
	// that's probably the most common case. If Create() fails then it
	// might be an update, e.g., someone tweaking their Envoy config
	// template.
	err = r.Create(ctx, &envoyConfig)
	if err != nil {
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
		For(&egwv1.LoadBalancer{}).
		Complete(r)
}

func envoyPodName(lb *egwv1.LoadBalancer) string {
	// this should let us see that:
	//  - it's an envoy
	//  - it's working for the lb named lb.Name
	return "envoy-" + lb.Name
}

// deploymentForLB returns a Deployment object that will launch an
// Envoy pod
func (r *LoadBalancerReconciler) deploymentForLB(lb *egwv1.LoadBalancer, spname *types.NamespacedName, envoyImage string) *appsv1.Deployment {
	labels := labelsForLB(lb.Name)

	multusConfig, err := json.Marshal([]map[string]interface{}{{
		"name":      spname.Name,
		"namespace": spname.Namespace,
		"ips":       []string{lb.Spec.PublicAddress}},
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
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{cniAnnotation: string(multusConfig)},
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
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
									Name:      "envoy-sidecar-bootstrap",
									MountPath: "/etc/envoy/bootstrap",
									ReadOnly:  true,
								},
							},
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
							Name: "envoy-sidecar-bootstrap",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "envoy-sidecar-bootstrap",
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

	// Set LB instance as the owner and controller
	ctrl.SetControllerReference(lb, dep, r.Scheme)
	return dep
}

func splitNSName(name string) (*types.NamespacedName, error) {
	parts := strings.Split(name, string(types.Separator))
	if len(parts) < 2 {
		return nil, fmt.Errorf("Malformed NamespaceName: %q", parts)
	}

	return &types.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
}

// labelsForLB returns the labels for selecting the resources
// belonging to the given CR name.
func labelsForLB(name string) map[string]string {
	return map[string]string{"app": "egw", "role": "proxy", "loadbalancer_cr": name}
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

func addRt(publicaddr *net.IPNet, multusint string) error {

	var rta netlink.Route

	ifa, err := net.InterfaceByName(multusint)
	if err != nil {
		return err
	}

	rta.LinkIndex = ifa.Index

	rta.Dst = publicaddr

	//FIXME error handling, if route add fails should be retried, requeue?
	netlink.RouteReplace(&rta)

	return nil
}

func delRt(publicaddr string) {

	var rta netlink.Route

	_, rta.Dst, _ = net.ParseCIDR(publicaddr)

	//FIXME error handling, if route add fails should be retried, requeue?

	netlink.RouteDel(&rta)

}

// addIpsetEntry adds the provided address and ports to the "egw-in"
// set. The last error to happen will be returned.
func addIpsetEntry(publicaddr string, ports []corev1.ServicePort) (err error) {
	for _, port := range ports {
		cmd := exec.Command("ipset", "-exist", "add", "egw-in", ipsetAddress(publicaddr, port))
		if cmdErr := cmd.Run(); cmdErr != nil {
			err = cmdErr
		}
	}
	return err
}

// delIpsetEntry deletes the provided address and ports from the
// "egw-in" set. The last error to happen will be returned.
func delIpsetEntry(publicaddr string, ports []corev1.ServicePort) (err error) {
	for _, port := range ports {
		cmd := exec.Command("ipset", "-exist", "del", "egw-in", ipsetAddress(publicaddr, port))
		if cmdErr := cmd.Run(); cmdErr != nil {
			err = cmdErr
		}
	}
	return err
}

// ipsetAddress formats an address and port how ipset likes them,
// e.g., "192.168.77.2,tcp:8088". From what I can tell, ipset accepts
// the protocol in upper-case and lower-case so we don't need to do
// anything with it here.
func ipsetAddress(publicaddr string, port corev1.ServicePort) string {
	return fmt.Sprintf("%s,%s:%d", publicaddr, port.Protocol, port.Port)
}

// configureBridge configures the bridge interface used to get packets
// from the Envoy pods to the customer endpoints. It's called
// "multus0" by default but that can be overridden in the
// ServicePrefix CR.
func (r *LoadBalancerReconciler) configureBridge(brname string, gateway *netlink.Addr) error {
	var err error

	_, err = r.ensureBridge(brname, gateway)
	if err != nil {
		r.Log.Error(err, "Multus", "unable to setup", brname)
	}

	err = ipsetSetCheck()
	if err == nil {
		r.Log.Info("IPset egw-in added")
	} else {
		r.Log.Info("IPset egw-in already exists")
	}

	err = ipTablesCheck(brname)
	if err == nil {
		r.Log.Info("IPtables setup for multus")
	} else {
		r.Log.Error(err, "iptables", "unable to setup", brname)
	}

	return err
}

// ensureBridge creates the bridge if it's not there and configures it in either case.
func (r *LoadBalancerReconciler) ensureBridge(brname string, gateway *netlink.Addr) (*netlink.Bridge, error) {

	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   brname,
			TxQLen: -1,
		},
	}

	// create the bridge if it doesn't already exist
	_, err := netlink.LinkByName(brname)
	if err != nil {
		err = netlink.LinkAdd(br)
		if err != nil && err != syscall.EEXIST {
			r.Log.Info("Multus Bridge could not add %q: %v", brname, err)
		}
	} else {
		r.Log.Info(brname + " already exists")
	}

	// bring the interface up
	if err := netlink.LinkSetUp(br); err != nil {
		return nil, err
	}

	// Proxy ARP is required when we use a device route for the default gateway
	_, err = sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/proxy_arp", brname), "1")
	if err != nil {
		return nil, err
	}

	// add the gateway address to the bridge interface
	err = netlink.AddrReplace(br, gateway)
	if err != nil {
		return nil, fmt.Errorf("could not add %v: to %v %w", gateway, br, err)
	}

	return br, nil
}

// ipsetSetCheck runs ipset to create the egw-in table.
func ipsetSetCheck() error {
	cmd := exec.Command("/usr/sbin/ipset", "create", "egw-in", "hash:ip,port")
	return cmd.Run()
}

// ipTablesCheck sets up the iptables rules.
func ipTablesCheck(brname string) error {
	var (
		cmd          *exec.Cmd
		err          error
		stdoutStderr []byte
	)

	cmd = exec.Command("/usr/sbin/iptables", "-C", "FORWARD", "-i", brname, "-m", "comment", "--comment", "multus bridge "+brname, "-j", "ACCEPT")
	err = cmd.Run()
	if err != nil {
		cmd = exec.Command("/usr/sbin/iptables", "-A", "FORWARD", "-i", brname, "-m", "comment", "--comment", "multus bridge "+brname, "-j", "ACCEPT")
		stdoutStderr, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("ERROR: %s\n", string(stdoutStderr))
			return err
		}
	}

	cmd = exec.Command("/usr/sbin/iptables", "-C", "FORWARD", "-m", "set", "--match-set", "egw-in", "dst,dst", "-j", "ACCEPT")
	err = cmd.Run()
	if err != nil {
		cmd = exec.Command("/usr/sbin/iptables", "-A", "FORWARD", "-m", "set", "--match-set", "egw-in", "dst,dst", "-j", "ACCEPT")
		stdoutStderr, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("ERROR: %s\n", string(stdoutStderr))
			return err
		}
	}

	return err
}

func (r *LoadBalancerReconciler) deleteTunnel(l logr.Logger, ep egwv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", ep.TunnelID)
	return egwexec.RunScript(l, script)
}

func (r *LoadBalancerReconciler) deleteService(l logr.Logger, groupID uint16, serviceID uint16) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service del %[1]d %[2]d", groupID, serviceID)
	return egwexec.RunScript(l, script)
}

func (r *LoadBalancerReconciler) configureTagging(l logr.Logger, ifname string) error {
	err := pfc.AddQueueDiscipline(ifname)
	if err != nil {
		return err
	}
	return pfc.AddFilter(ifname, "ingress", "tag_rx")
}

// cleanupPFC undoes the PFC setup that we did for this lb.
func (r *LoadBalancerReconciler) cleanupPFC(l logr.Logger, lb *egwv1.LoadBalancer, acct *egwv1.Account, prefix *egwv1.ServicePrefix) error {
	// remove IPSet entry
	delIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts)

	// remove route
	publicaddr := prefix.Spec.Subnet
	if prefix.Spec.Aggregation == "default" {
		publicaddr = lb.Spec.PublicAddress + prefix.Spec.Aggregation
	}
	delRt(publicaddr)

	// remove the endpoint PFC "services"
	r.deleteService(l, acct.Spec.GroupID, lb.Spec.ServiceID)

	// remove the PFC tunnels
	for _, tunnel := range lb.Status.GUETunnelEndpoints {
		r.deleteTunnel(l, tunnel)
	}

	return nil
}
