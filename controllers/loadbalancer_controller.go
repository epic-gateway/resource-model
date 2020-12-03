package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	"github.com/vishvananda/netlink"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	"gitlab.com/acnodal/egw-resource-model/internal/pfc"
)

const (
	envoyImage    = "registry.gitlab.com/acnodal/envoy-for-egw:adamd-dev"
	gitlabSecret  = "gitlab"
	cniAnnotation = "k8s.v1.cni.cncf.io/networks"
	tunnelAuth    = "fredfredfredfred" // FIXME: this should be in one of our CRs
)

var (
	tunnelID uint32 = 42 // FIXME: this needs to be managed more robustly than just a variable
)

// LoadBalancerReconciler reconciles a LoadBalancer object
type LoadBalancerReconciler struct {
	client.Client
	Log     logr.Logger
	Scheme  *runtime.Scheme
	lbAddrs map[string]*net.IPNet //lb.Namespace+lb.Name -> IPNet
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *LoadBalancerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	done := ctrl.Result{Requeue: false}
	tryAgain := ctrl.Result{RequeueAfter: 10 * time.Second}
	ctx := context.TODO()
	log := r.Log.WithValues("loadbalancer", req.NamespacedName)

	log.Info("reconciling")

	r.lbAddrs = map[string]*net.IPNet{}

	// read the object that caused the event
	lb := &egwv1.LoadBalancer{}
	err = r.Get(ctx, req.NamespacedName, lb)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("resource not found. Ignoring since object must be deleted")
			return done, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource, will retry")
		return tryAgain, err
	}

	// read the ServiceGroup that owns this LB
	sg := &egwv1.ServiceGroup{}
	sgname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Spec.ServiceGroup}
	err = r.Get(ctx, sgname, sg)
	if err != nil {
		log.Error(err, "Failed to find owning service group", "name", sgname)
		return done, err
	}

	// the ServiceGroup points to the ServicePrefix
	spname, err := splitNSName(sg.Spec.ServicePrefix)
	if err != nil {
		log.Error(err, "Failed to look up ServicePrefix", "name", spname)
		return done, err
	}

	// read the ServicePrefix that determines the interface that we'll use
	prefix := &egwv1.ServicePrefix{}
	err = r.Get(ctx, *spname, prefix)
	if err != nil {
		log.Error(err, "Failed to find owning service prefix", "name", spname)
		return done, err
	}

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: envoyPodName(lb), Namespace: lb.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForLB(lb, spname)

		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return done, err
		}
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return done, err
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
		log.Error(err, "Failed to configure multus bridge", "name", spname)
		return done, err
	}

	// configure the GUE service
	for _, ep := range lb.Spec.Endpoints {
		// Add GUE ingress address/port to the endpoint
		gueEp, err := r.setGUEIngressAddress(ctx, lb, &ep)
		if err != nil {
			log.Error(err, "Patching LB status")
			return done, err
		}

		// configure the GUE tunnel
		err = r.configureTunnel(log, gueEp)
		if err != nil {
			log.Error(err, "configuring GUE tunnel")
			return done, err
		}

		err = r.configureService(log, ep, lb.Status.ProxyIfindex, gueEp.TunnelID, lb.Spec.GUEKey)
		if err != nil {
			log.Error(err, "configuring GUE service")
			return done, err
		}
	}

	// Setup host routing

	if prefix.Spec.Aggregation != "default" {

		_, publicaddr, _ := net.ParseCIDR(lb.Spec.PublicAddress + prefix.Spec.Aggregation)

		err = addRt(publicaddr, prefix.Spec.MultusBridge)
		if err != nil {
			log.Error(err, "adding route to multus interface")
		}

		r.lbAddrs[(lb.Namespace + lb.Name)] = publicaddr

	} else {

		_, publicaddr, _ := net.ParseCIDR(prefix.Spec.Subnet)

		err = addRt(publicaddr, prefix.Spec.MultusBridge)
		if err != nil {
			log.Error(err, "adding route to multus interface")
		}

		r.lbAddrs[(lb.Namespace + lb.Name)] = publicaddr

	}

	//Open host ports by updating IPSET tables
	addIpsetEntry(lb.Spec.PublicAddress, lb.Spec.PublicPorts)

	return done, nil
}

func envoyPodName(lb *egwv1.LoadBalancer) string {
	// this should let us see that:
	//  - it's an envoy
	//  - it's working for the lb named lb.Name
	return "envoy-" + lb.Name
}

// deploymentForLB returns a Deployment object that will launch an
// Envoy pod
func (r *LoadBalancerReconciler) deploymentForLB(lb *egwv1.LoadBalancer, spname *types.NamespacedName) *appsv1.Deployment {
	var (
		privileged  bool = true
		escalatable bool = true
	)

	labels := labelsForLB(lb.Name)
	replicas := int32(1)

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
	envoyOverrides := fmt.Sprintf("node: {cluster: %s, id: %s/%s}", lb.Namespace, lb.Namespace, lb.Name)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lb.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
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
							Args:            []string{"envoy", "--config-path", "/etc/envoy/envoy.yaml", "--config-yaml", envoyOverrides},
							SecurityContext: &corev1.SecurityContext{
								Privileged:               &privileged,
								AllowPrivilegeEscalation: &escalatable,
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"NET_ADMIN"},
								},
							},
						},
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: gitlabSecret},
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

func addIpsetEntry(publicaddr string, ports []corev1.ServicePort) {

	for _, port := range ports {
		cmd := exec.Command("ipset", "-exist", "add", "egw-in", publicaddr+","+strconv.Itoa(int(port.Port)))
		err := cmd.Run()
		if err != nil {
			println(err)
		}

	}

}
func delIpsetEntry(publicaddr string, ports []corev1.ServicePort) {

	for _, port := range ports {
		cmd := exec.Command("ipset", "-exist", "del", "egw-in", publicaddr+":"+strconv.Itoa(int(port.Port)))
		err := cmd.Run()
		if err != nil {
			println(err)
		}

	}

}

// SetupWithManager sets up this controller to work with the mgr.
func (r *LoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.LoadBalancer{}).
		Complete(r)
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

// setGUEIngressAddress sets the GUEAddress/Port fields of the
// LoadBalancer status. This is used by PureLB to open a GUE tunnel
// back to the EGW. It returns a GUETunnelEndpoint, either the newly
// created one or the existing one. If error is non-nil then the
// GUETunnelEndpoint is invalid.
func (r *LoadBalancerReconciler) setGUEIngressAddress(ctx context.Context, lb *egwv1.LoadBalancer, ep *egwv1.LoadBalancerEndpoint) (egwv1.GUETunnelEndpoint, error) {
	var (
		err         error
		gueEndpoint egwv1.GUETunnelEndpoint
	)

	// We set up a tunnel to each client node that has an endpoint that
	// belongs to this service. If we already have a tunnel to this
	// endpoint's node (i.e., two endpoints in the same service on the
	// same node) then we don't need to do anything
	if gueEndpoint, exists := lb.Status.GUETunnelEndpoints[ep.NodeAddress]; exists {
		log.Info("EP node already has a tunnel", "endpoint", ep)
		return gueEndpoint, nil
	}

	// fetch the NodeConfig; it tells us the GUEEndpoint for this node
	nc := &egwv1.NodeConfig{}
	err = r.Get(ctx, types.NamespacedName{Name: "default", Namespace: "egw"}, nc)
	if err != nil {
		return gueEndpoint, err
	}
	nc.Spec.Base.GUEEndpoint.DeepCopyInto(&gueEndpoint)
	gueEndpoint.TunnelID = tunnelID

	// FIXME: allocate this instead of just incrementing
	tunnelID++

	// prepare a patch to set this node's tunnel endpoint in the LB status
	patchBytes, err := json.Marshal(egwv1.LoadBalancer{Status: egwv1.LoadBalancerStatus{GUETunnelEndpoints: map[string]egwv1.GUETunnelEndpoint{ep.NodeAddress: gueEndpoint}}})
	if err != nil {
		return gueEndpoint, err
	}
	log.Info(string(patchBytes))

	// apply the patch
	err = r.Status().Patch(ctx, lb, client.RawPatch(types.MergePatchType, patchBytes))
	if err != nil {
		log.Error("patching LB status", "lb", lb, "error", err)
		return gueEndpoint, err
	}

	log.Info("LB status patched", "lb", lb)
	return gueEndpoint, err
}

func (r *LoadBalancerReconciler) configureTunnel(l logr.Logger, ep egwv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel get %[1]d | grep TUN *%[2]s || /opt/acnodal/bin/cli_tunnel set %[1]d %[2]s %[3]d 0 0", ep.TunnelID, ep.Address, ep.Port.Port)
	l.Info(script)
	cmd := exec.Command("/bin/sh", "-c", script)
	return cmd.Run()
}

func (r *LoadBalancerReconciler) configureService(l logr.Logger, ep egwv1.LoadBalancerEndpoint, ifindex int, tunnelID uint32, tunnelKey uint32) error {
	// split the tunnelKey into its parts: groupId in the upper 16 bits
	// and serviceId in the lower 16
	var groupID uint16 = uint16(tunnelKey & 0xffff)
	var serviceID uint16 = uint16(tunnelKey >> 16)

	script := fmt.Sprintf("/opt/acnodal/bin/cli_service get all | grep 'ENCAP.*tcp %[5]s:%[6]d' || /opt/acnodal/bin/cli_service set-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	l.Info(script)
	cmd := exec.Command("/bin/sh", "-c", script)
	return cmd.Run()
}

func (r *LoadBalancerReconciler) configureTagging(l logr.Logger, ifname string) error {
	err := pfc.AddQueueDiscipline(ifname)
	if err != nil {
		return err
	}
	return pfc.AddFilter(ifname, "ingress", "tag_rx")
}
