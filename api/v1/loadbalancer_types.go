package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// LoadBalancerSpec defines the desired state of LoadBalancer
type LoadBalancerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ServiceGroup is the name of the ServiceGroup to which this
	// LoadBalancer belongs.
	ServiceGroup string `json:"service-group"`

	// PublicAddress is the publicly-visible IP address for this LB.
	PublicAddress string `json:"public-address"`

	// Ports is the set of ports on which this LB will listen.
	Ports []int `json:"ports"`
}

// LoadBalancerEndpoint represents one endpoint on a customer cluster.
type LoadBalancerEndpoint struct {
	// Address is the IP address for this endpoint.
	Address string `json:"address"`

	// Port is the port on which this endpoint listens.
	Port int `json:"port"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Endpoints []LoadBalancerEndpoint `json:"endpoints"`
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
