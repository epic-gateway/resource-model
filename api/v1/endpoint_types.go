package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// EndpointFinalizerName is the name of the finalizer that cleans up
	// when an Endpoint CR is deleted.
	// FIXME: I don't think that this is a valid finalizer name. See the xds controller for a better one.
	EndpointFinalizerName string = "egw.acnodal.io/endpointFinalizer"

	// OwningLoadBalancerLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// LoadBalancer they belong.
	OwningLoadBalancerLabel string = "owningLB"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// EndpointSpec defines the desired state of Endpoint. It represents
// one pod endpoint on a customer cluster.
type EndpointSpec struct {
	// LoadBalancer is the name of the LoadBalancer to which this
	// endpoint belongs.
	// FIXME: remove this since it's redundant with OwningLoadBalancerLabel.
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
	// The ProxyIfindex in the LoadBalancer Status is canonical but we
	// cache it here so we can cleanup the PFC service without having to
	// lookup the LB since it might have been deleted.
	ProxyIfindex int `json:"proxy-ifindex,omitempty"`

	// The GUEKey in the LoadBalancer Spec is canonical but we cache it
	// here so we can cleanup the PFC service without having to lookup
	// the LB since it might have been deleted.
	GUEKey uint32 `json:"gue-key,omitempty"`

	// The TunnelID in the LoadBalancer Status is canonical but we cache
	// it here so we can cleanup the PFC service without having to
	// lookup the LB since it might have been deleted.
	TunnelID uint32 `json:"tunnel-id,omitempty"`
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
