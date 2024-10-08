package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccountSpec defines the desired state of Account
type AccountSpec struct {
	// ProxyLimit defines how many proxies can be created in this
	// account.
	// +kubebuilder:default=20
	ProxyLimit int `json:"proxyLimit,omitempty"`
}

// AccountStatus defines the observed state of Account
type AccountStatus struct {
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

// AccountNamespace returns the namespace for the provided account
// name.
func AccountNamespace(acctName string) string {
	return UserNamespacePrefix + acctName
}
