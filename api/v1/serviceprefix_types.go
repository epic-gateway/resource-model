package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// ServicePrefixSpec defines the desired state of ServicePrefix
type ServicePrefixSpec struct {
	Subnet string `json:"subnet"`
	Pool   string `json:"pool"`
	// +kubebuilder:default=default
	Aggregation string `json:"aggregation,omitempty"`
	// +kubebuilder:default=multus0
	MultusBridge string `json:"multus-bridge,omitempty"`
}

// ServicePrefixStatus defines the observed state of ServicePrefix
type ServicePrefixStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServicePrefix represents a pool of IP addresses. The EGW web
// service will allocate addresses from the set of ServicePrefixes.
type ServicePrefix struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServicePrefixSpec   `json:"spec,omitempty"`
	Status ServicePrefixStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServicePrefixList contains a list of ServicePrefix
type ServicePrefixList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServicePrefix `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServicePrefix{}, &ServicePrefixList{})
}
