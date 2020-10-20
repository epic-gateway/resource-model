package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/go-logr/logr"
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
)

const (
	envoyImage    = "registry.gitlab.com/acnodal/envoy-for-egw:latest"
	gitlabSecret  = "gitlab"
	cniAnnotation = "k8s.v1.cni.cncf.io/networks"
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
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=pods,verbs=get;list;

// Reconcile is the core of this controller. It gets requests from the
// controller-runtime and figures out what to do with them.
func (r *LoadBalancerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	result := ctrl.Result{}
	ctx := context.TODO()
	log := r.Log.WithValues("loadbalancer", req.NamespacedName)

	r.lbAddrs = map[string]*net.IPNet{}

	// read the object that caused the event
	lb := &egwv1.LoadBalancer{}
	err := r.Get(ctx, req.NamespacedName, lb)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("resource not found. Ignoring since object must be deleted")
			return result, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get resource, will retry")
		return result, err
	}

	// Setup host routing

	// FIXME multus interface name is hardcoded, we should be getting it from netattachdef

	var multusint = "multus0"

	prefix := &egwv1.ServicePrefix{}

	//FIXME - Static defination for Service Prefix CR

	r.Get(ctx, types.NamespacedName{Name: "default", Namespace: "egw"}, prefix)

	if prefix.Spec.Aggregation != "default" {

		_, publicaddr, _ := net.ParseCIDR(lb.Spec.PublicAddress + prefix.Spec.Aggregation)

		addRt(publicaddr, multusint)

		r.lbAddrs[(lb.Namespace + lb.Name)] = publicaddr

	} else {

		_, publicaddr, _ := net.ParseCIDR(prefix.Spec.Subnet)

		addRt(publicaddr, multusint)

		r.lbAddrs[(lb.Namespace + lb.Name)] = publicaddr

	}

	//Open host ports by updating IPSET tables
	addIpset(lb.Spec.PublicAddress, lb.Spec.PublicPorts)

	// Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: lb.Name, Namespace: lb.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		dep := r.deploymentForLB(lb)

		log.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.Create(ctx, dep)
		if err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return result, err
		}

		// Deployment created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil

	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return result, err
	}

	return result, nil
}

// deploymentForLB returns a Deployment object that will launch an
// Envoy pod
func (r *LoadBalancerReconciler) deploymentForLB(lb *egwv1.LoadBalancer) *appsv1.Deployment {
	labels := labelsForLB(lb.Name)
	replicas := int32(1)

	multusConfig, err := json.Marshal([]map[string]interface{}{{
		// FIXME: need to add parameters to tell Multus which netdef to use
		"name":      "default",
		"namespace": "egw",
		"ips":       []string{lb.Spec.PublicAddress}},
	})
	if err != nil {
		return nil
	}

	// format a fragment of Envoy config yaml that will set the dynamic
	// command-line overrides that differentiate this instance of Envoy
	// from the others.
	envoyOverrides := fmt.Sprintf("node: {cluster: %s, id: %s/%s}", lb.Namespace, lb.Namespace, lb.Name)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lb.Name,
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
							Name:            lb.Name,
							Ports:           portsToPorts(lb.Spec.PublicPorts),
							Command:         []string{"/docker-entrypoint.sh"},
							Args:            []string{"envoy", "--config-path", "/etc/envoy/envoy.yaml", "--config-yaml", envoyOverrides},
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

// labelsForLB returns the labels for selecting the resources
// belonging to the given CR name.
func labelsForLB(name string) map[string]string {
	return map[string]string{"app": "egw", "loadbalancer_cr": name}
}

func portsToPorts(rawPorts []int) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, len(rawPorts))
	for i, port := range rawPorts {
		ports[i] = corev1.ContainerPort{ContainerPort: int32(port)}
	}
	return ports
}

func addRt(publicaddr *net.IPNet, multusint string) {

	var rta netlink.Route

	ifa, _ := net.InterfaceByName(multusint)

	rta.LinkIndex = ifa.Index

	rta.Dst = publicaddr

	//FIXME error handling, if route add fails should be retried, requeue?
	netlink.RouteReplace(&rta)

}

func delRt(publicaddr string) {

	var rta netlink.Route

	_, rta.Dst, _ = net.ParseCIDR(publicaddr)

	//FIXME error handling, if route add fails should be retried, requeue?

	netlink.RouteDel(&rta)

}

func addIpset(publicaddr string, rawPorts []int) {

	for _, ports := range rawPorts {
		port := strconv.Itoa(ports)
		cmd := exec.Command("ipset", "add", "egw-in", publicaddr+","+port)
		err := cmd.Run()
		if err != nil {
			println(err)
		}

	}

}
func delIpset(publicaddr string, rawPorts []int) {

	for _, ports := range rawPorts {

		port := strconv.Itoa(ports)
		cmd := exec.Command("ipset", "del", "egw-in", publicaddr+":"+port)
		err := cmd.Run()
		if err != nil {
			println(err)
		}

	}

}

// SetupWithManager sets up this reconciler to be managed.
func (r *LoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.LoadBalancer{}).
		Complete(r)
}
