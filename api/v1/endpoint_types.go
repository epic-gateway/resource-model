package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// EndpointFinalizerName is the name of the finalizer that cleans up
	// when an Endpoint CR is deleted.
	EndpointFinalizerName string = "egw.acnodal.io/endpointFinalizer"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// EndpointSpec defines the desired state of Endpoint. It represents
// one pod endpoint on a customer cluster.
type EndpointSpec struct {
	// LoadBalancer is the name of the LoadBalancer to which this
	// endpoint belongs.
	LoadBalancer string `json:"load-balancer"`

	// Address is the IP address for this endpoint.
	Address string `json:"address"`

	// NodeAddress is the IP address of the node on which this endpoint
	// is running. We use it to set up a GUE tunnel from the EGW to the
	// node.
	NodeAddress string `json:"node-address"`

	// Port is the port on which this endpoint listens.
	Port corev1.EndpointPort `json:"port"`
}

// EndpointStatus defines the observed state of Endpoint
type EndpointStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Endpoint is the Schema for the endpoints API
type Endpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointSpec   `json:"spec,omitempty"`
	Status EndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EndpointList contains a list of Endpoint
type EndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Endpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Endpoint{}, &EndpointList{})
}
