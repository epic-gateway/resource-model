package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConfigName is the name of the EGW configuration singleton. Its
	// namespace is defined in namespaces.go.
	ConfigName = "egw"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// Node is the config for one node.
type Node struct {
	// +kubebuilder:default={"enp1s0"}
	IngressNICs []string `json:"gue-ingress-nics,omitempty"`

	// +kubebuilder:default={"egw-port":{"port":6080,"protocol":"UDP","appProtocol":"gue"}}
	GUEEndpoint GUETunnelEndpoint `json:"gue-endpoint,omitempty"`
}

// EGWSpec defines the desired state of EGW
type EGWSpec struct {
	// NodeBase is the "base" configuration for all nodes in the
	// cluster.
	NodeBase Node `json:"base"`
}

// EGWStatus defines the observed state of EGW
type EGWStatus struct {
	// CurrentGroupID stores the most-recently-allocated value of the
	// account-level GUE group id. Clients should read the CR, calculate
	// the next value and then write that back using Update() and not
	// Patch(). If the write succeeds then they own that value. If not
	// then they need to try again.
	CurrentGroupID uint16 `json:"current-group-id"`

	// CurrentTunnelID stores the most-recently-allocated tunnel ID. See
	// the comments on CurrentGroupID for notes on how to use this
	// field.
	CurrentTunnelID uint32 `json:"current-tunnel-id"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EGW is the Schema for the egws API
type EGW struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EGWSpec   `json:"spec,omitempty"`
	Status EGWStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EGWList contains a list of EGW
type EGWList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EGW `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EGW{}, &EGWList{})
}
