package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ServiceGroupSpec defines the desired state of ServiceGroup
type ServiceGroupSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Account is the UID of the Account that is responsible for this
	// ServiceGroup.
	Account string `json:"account"`
}

// ServiceGroupStatus defines the observed state of ServiceGroup
type ServiceGroupStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ServiceGroup is the Schema for the servicegroups API
type ServiceGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceGroupSpec   `json:"spec,omitempty"`
	Status ServiceGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceGroupList contains a list of ServiceGroup
type ServiceGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceGroup{}, &ServiceGroupList{})
}
