package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// traffic.
	Endpoints []LoadBalancerEndpoint `json:"endpoints,omitempty"`

	// GUEKey is used with the account-level GUEKey to set up this
	// service's GUE tunnels between the EGW and the client cluster. The
	// account-level GUEKey is 16 bits but this GUEKey is 32 bits
	// because it contains *both* the account key (in the upper 16 bits)
	// and the service key (in the lower 16 bits). It should not be set
	// in the YAML manifest - a webhook will fill it in when the CR is
	// created.
	GUEKey uint32 `json:"gue-key,omitempty"`
}

// LoadBalancerEndpoint represents one endpoint on a customer cluster.
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

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// GUEAddress is the EGW's GUE tunnel endpoint address for this load
	// balancer.
	GUEAddress string `json:"gue-address"`
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
