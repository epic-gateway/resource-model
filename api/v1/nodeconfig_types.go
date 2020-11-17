package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Node is the config for one node.
type Node struct {
	IngressNICs []string `json:"ingress-nics"`

	// PublicIngressAddress is the IP address that PureLB clients use to
	// establish GUE tunnels to this node. It must be publicly visible
	// so clients can send packets to it from anywhere on the internet.
	GUEIngressAddress string `json:"gue-ingress-address"`
}

// NodeConfigSpec defines the desired state of NodeConfig
type NodeConfigSpec struct {
	// Base is the default for all nodes.
	Base Node `json:"base"`
}

// NodeConfigStatus defines the observed state of NodeConfig
type NodeConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NodeConfig is the Schema for the nodeconfigs API
type NodeConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeConfigSpec   `json:"spec,omitempty"`
	Status NodeConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeConfigList contains a list of NodeConfig
type NodeConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeConfig{}, &NodeConfigList{})
}
