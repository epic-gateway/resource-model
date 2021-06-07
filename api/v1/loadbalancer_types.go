package v1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// LoadbalancerFinalizerName is the name of the finalizer that
	// cleans up when a LoadBalancer CR is deleted.
	LoadbalancerFinalizerName string = "loadbalancer-finalizer.controller-manager.acnodal.io"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// LoadBalancerSpec defines the desired state of LoadBalancer
type LoadBalancerSpec struct {
	// PublicAddress is the publicly-visible IP address for this LB.
	PublicAddress string `json:"public-address"`

	// PublicPorts is the set of ports on which this LB will listen.
	PublicPorts []corev1.ServicePort `json:"public-ports"`

	// ServiceID is used with the account-level GroupID to set up this
	// service's GUE tunnels between the EPIC and the client cluster. It
	// should not be set by the client - a webhook fills it in when the
	// CR is created.
	ServiceID uint16 `json:"service-id,omitempty"`

	// TunnelKey authenticates clients with the EPIC. It must be a
	// base64-encoded 128-bit value. If not present, this will be filled
	// in by the defaulting webhook.
	TunnelKey string `json:"tunnel-key,omitempty"`

	// EnvoyTemplate is the template that will be used to configure
	// Envoy for the load balancers that belong to this LBServiceGroup. It
	// can be provided by the user, but if not it will be copied from
	// the owning LBServiceGroup.
	EnvoyTemplate *marin3r.EnvoyConfigSpec `json:"envoy-template,omitempty"`

	// UpstreamClusters is a list of the names of the upstream clusters
	// (currently only PureLB but maybe other types in the future) that
	// this LB serves. The string must be the cluster-id that is passed
	// in the announce and withdraw web service methods.
	UpstreamClusters []string `json:"upstream-clusters,omitempty"`
}

// GUETunnelEndpoint is an Endpoint on the EPIC.
type GUETunnelEndpoint struct {
	// Address is the IP address on the EPIC for this endpoint.
	Address string `json:"epic-address,omitempty"`

	// Port is the port on which this endpoint listens.
	// +kubebuilder:default={"port":6080,"protocol":"UDP","appProtocol":"gue"}
	Port corev1.EndpointPort `json:"epic-port,omitempty"`

	// TunnelID is used to route traffic to the correct tunnel.
	TunnelID uint32 `json:"tunnel-id,omitempty"`
}

// ProxyInterfaceInfo contains information about the Envoy proxy pod's
// network interfaces.
type ProxyInterfaceInfo struct {
	// Port is the port on which this endpoint listens.
	// +kubebuilder:default={"port":6080,"protocol":"UDP","appProtocol":"gue"}
	Port corev1.EndpointPort `json:"epic-port,omitempty"`

	// Index is the ifindex of the Envoy proxy pod's veth interface on
	// the CRI side of this service's proxy pod. In other words, it's
	// the end of the veth that's inside the container (i.e., on the
	// other side from the end that's attached to the multus bridge).
	Index int `json:"index,omitempty"`

	// Name is the name of the interface whose index is ProxyIfindex.
	Name string `json:"name,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// GUETunnelEndpoints is a map from client node addresses to public
	// GUE tunnel endpoints on the EPIC. The map key is a client node
	// address and must be one of the node addresses in the Spec
	// Endpoints slice. The value is a GUETunnelEndpoint that describes
	// the public IP and port to which the client can send tunnel ping
	// packets.
	GUETunnelEndpoints map[string]GUETunnelEndpoint `json:"gue-tunnel-endpoints,omitempty"`

	// ProxyInterfaces contains information about the Envoy proxy pods'
	// network interfaces. The map key is the node IP address. It's
	// filled in by the python setup-network daemon and used by the
	// loadbalancer controller.
	ProxyInterfaces map[string]ProxyInterfaceInfo `json:"proxy-if-info,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=lb;lbs
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.public-address",name=Public Address,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.owning-lbservicegroup",name=Service Group,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.owning-serviceprefix",name=Service Prefix,type=string

// LoadBalancer is the Schema for the loadbalancers API
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoadBalancerList contains a list of LoadBalancer
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}

// NamespacedName returns a NamespacedName object filled in with this
// Object's name info.
func (lb *LoadBalancer) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: lb.Namespace, Name: lb.Name}
}

// AddUpstream adds "clusterId" as an upstream cluster to this
// LB. Returns an error if the LB already contained the cluster, nil
// if it didn't.
func (lb *LoadBalancer) AddUpstream(clusterID string) error {
	if lb.ContainsUpstream(clusterID) {
		return fmt.Errorf("LB %s already contains upstream %s", lb.NamespacedName(), clusterID)
	}
	lb.Spec.UpstreamClusters = append(lb.Spec.UpstreamClusters, clusterID)
	return nil
}

// ContainsUpstream indicates whether "contains" is already registered
// as an upstream cluster. Returns true if it is, false if it isn't.
func (lb *LoadBalancer) ContainsUpstream(contains string) bool {
	for _, upstream := range lb.Spec.UpstreamClusters {
		if contains == upstream {
			return true
		}
	}

	return false
}

// RemoveUpstream removes "clusterId" as an upstream cluster from this
// LB. Returns nil if the LB already contained the cluster, an error
// if it didn't.
func (lb *LoadBalancer) RemoveUpstream(clusterID string) error {
	for i, upstream := range lb.Spec.UpstreamClusters {
		if clusterID == upstream {
			// https://stackoverflow.com/a/37335777/5967960
			c := lb.Spec.UpstreamClusters
			c[i] = c[len(c)-1]
			lb.Spec.UpstreamClusters = c[:len(c)-1]
			return nil
		}
	}

	return fmt.Errorf("Upstream %s not found", clusterID)
}

// LoadBalancerName returns the name that we use in the LoadBalancer
// custom resource. This is a combo of the ServicePrefix name and the
// "raw" load balancer name. We need to smash them together because
// one customer might have two or more LBs with the same name, but
// belonging to different service prefixes, and a customer's LBs all
// live in the same k8s namespace so we need to make the service group
// name into an ersatz namespace.
func LoadBalancerName(sgName string, lbName string, canBeShared bool) (name string) {

	// The group name is a sorta-kinda "namespace" because two services
	// with the same name in the same group will be shared but two
	// services with the same name in different groups will be
	// independent. Since the loadbalancer CRs for both services live in
	// the same k8s namespace, to make this work we prepend the group
	// name to the service name. This way two services with the same
	// name won't collide if they belong to different groups.
	name = sgName + "-" + lbName

	// If this LB can not be shared then give it a random readable name
	// so it doesn't collide with other LBs in the group that might be
	// created by other services with the same name. If the LB can be
	// shared then leave its name alone so other PureLB services with
	// the same name can find it. When they try to create their services
	// they'll get a 409 Conflict response that points them at this
	// service.
	if !canBeShared {
		raw := make([]byte, 8, 8)
		_, _ = rand.Read(raw)
		name += "-" + hex.EncodeToString(raw)
	}

	return name
}

func init() {
	SchemeBuilder.Register(&LoadBalancer{}, &LoadBalancerList{})
}
