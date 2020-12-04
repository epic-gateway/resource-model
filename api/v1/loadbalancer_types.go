package v1

import (
	"fmt"

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
	// ServiceGroup is the name of the ServiceGroup to which this
	// LoadBalancer belongs.
	ServiceGroup string `json:"service-group"`

	// PublicAddress is the publicly-visible IP address for this LB.
	PublicAddress string `json:"public-address"`

	// PublicPorts is the set of ports on which this LB will listen.
	PublicPorts []corev1.ServicePort `json:"public-ports"`

	// Endpoints are the customer-cluster endpoints to which we send
	// traffic. This field is defaulted to "[]" in the admission webhook
	// so we can use and "add" JSONPatch to insert endpoints.
	Endpoints []LoadBalancerEndpoint `json:"endpoints"`

	// GUEKey is used with the account-level GUEKey to set up this
	// service's GUE tunnels between the EGW and the client cluster. The
	// account-level GUEKey is 16 bits but this GUEKey is 32 bits
	// because it contains *both* the account key (in the upper 16 bits)
	// and the service key (in the lower 16 bits). It should not be set
	// in the YAML manifest - a webhook fills it in when the CR is
	// created.
	GUEKey uint32 `json:"gue-key,omitempty"`
}

// ValidateEndpoints checks that each endpoint is unique.
func (r *LoadBalancerSpec) ValidateEndpoints() error {
	for i1, ep1 := range r.Endpoints {
		for i2, ep2 := range r.Endpoints {
			if i1 != i2 && ep1 == ep2 {
				return fmt.Errorf("duplicate endpoints. endpoints at index %d and %d are the same", i1, i2)
			}
		}
	}
	return nil
}

// AddEndpoint adds an endpoint to the LoadBalancerSpec but returns an
// error if it matches an existing one.
func (r *LoadBalancerSpec) AddEndpoint(ep LoadBalancerEndpoint) error {
	// Fail if the new endpoint matches an existing one
	for i1, ep1 := range r.Endpoints {
		if ep1 == ep {
			return fmt.Errorf("duplicate endpoints. new endpoint is the same as index %d", i1)
		}
	}

	// The new endpoint is unique so add it to the spec
	r.Endpoints = append(r.Endpoints, ep)
	return nil
}

// LoadBalancerEndpoint is an Endpoint on the client cluster.
type LoadBalancerEndpoint struct {
	// Address is the IP address for this endpoint.
	Address string `json:"address"`

	// NodeAddress is the IP address of the node on which this endpoint
	// is running. We use it to set up a GUE tunnel from the EGW to the
	// node.
	NodeAddress string `json:"node-address"`

	// Port is the port on which this endpoint listens.
	Port corev1.EndpointPort `json:"port"`
}

// GUETunnelEndpoint is an Endpoint on the EGW.
type GUETunnelEndpoint struct {
	// Address is the IP address on the EGW for this endpoint.
	Address string `json:"egw-address,omitempty"`

	// Port is the port on which this endpoint listens.
	// +kubebuilder:default={"port":4242,"appProtocol":"acnodal.io/gue-tunnel"}
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
