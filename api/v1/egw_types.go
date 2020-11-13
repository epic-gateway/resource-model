package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// EGWSpec defines the desired state of EGW
type EGWSpec struct {
}

// EGWStatus defines the observed state of EGW
type EGWStatus struct {
	// CurrentAccountGUEKey stores the most-recently-allocated value of
	// the account-level GUE key. Clients should read the CR, calculate
	// the next value and then write the next value back. If the write
	// succeeds then they can use the current value. If not then they
	// need to try again.
	CurrentAccountGUEKey uint16 `json:"next-gue-key"`
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
