package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LoadbalancerFinalizerName is the name of the finalizer that
	// cleans up when a LoadBalancer CR is deleted.
	LoadbalancerFinalizerName string = "egw.acnodal.io/loadBalancerFinalizer"
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
	// service's GUE tunnels between the EGW and the client cluster. It
	// should not be set by the client - a webhook fills it in when the
	// CR is created.
	ServiceID uint16 `json:"service-id,omitempty"`
}

// GUETunnelEndpoint is an Endpoint on the EGW.
type GUETunnelEndpoint struct {
	// Address is the IP address on the EGW for this endpoint.
	Address string `json:"egw-address,omitempty"`

	// Port is the port on which this endpoint listens.
	// +kubebuilder:default={"port":6080,"protocol":"UDP","appProtocol":"gue"}
	Port corev1.EndpointPort `json:"egw-port,omitempty"`

	// TunnelID is used to route traffic to the correct tunnel.
	TunnelID uint32 `json:"tunnel-id,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// GUETunnelEndpoints is a map from client node addresses to public
	// GUE tunnel endpoints on the EGW. The map key is a client node
	// address and must be one of the node addresses in the Spec
	// Endpoints slice. The value is a GUETunnelEndpoint that describes
	// the public IP and port to which the client can send tunnel ping
	// packets.
	GUETunnelEndpoints map[string]GUETunnelEndpoint `json:"gue-tunnel-endpoints,omitempty"`

	// ProxyIfindex is the ifindex of the Envoy proxy pod's veth
	// interface on the docker side of this service's proxy pod. In
	// other words, it's the end of the veth that's inside the container
	// (i.e., on the other side from the end that's attached to the
	// multus bridge). It's filled in by the python setup-network daemon
	// and used by the loadbalancer controller.
	ProxyIfindex int `json:"proxy-ifindex,omitempty"`

	// ProxyIfname is the name of the interface whose index is
	// ProxyIfindex.
	ProxyIfname string `json:"proxy-ifname,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=lb;lbs
// +kubebuilder:subresource:status

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

func init() {
	SchemeBuilder.Register(&LoadBalancer{}, &LoadBalancerList{})
}
