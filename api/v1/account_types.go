package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// AccountSpec defines the desired state of Account
type AccountSpec struct {
	// GUEKey is used with a service's GUEKey to set up GUE tunnels
	// between the EGW and the client cluster for that service. It
	// should not be set in the YAML manifest - the controller manager
	// will fill in the value when the object is created.
	GUEKey uint16 `json:"gue-key,omitempty"`
}

// AccountStatus defines the observed state of Account
type AccountStatus struct {
	// CurrentAccountGUEKey stores the most-recently-allocated value of
	// GUE key for services in this account. See the comments on
	// CurrentAccountGUEKey in the EGW CR for notes on how to use this
	// field.
	CurrentServiceGUEKey uint16 `json:"current-gue-key"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Account represents a business relationship between Acnodal and a
// third party. Since we know who we are, Account stores info about
// the third party and any info about our relationship that influences
// how the system operates.
type Account struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AccountSpec   `json:"spec,omitempty"`
	Status AccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AccountList contains a list of Account
type AccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Account `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Account{}, &AccountList{})
}
