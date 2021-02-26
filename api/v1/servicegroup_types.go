package v1

import (
	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// ServiceGroupSpec defines the desired state of ServiceGroup
type ServiceGroupSpec struct {
	// EnvoyImage is the name of the Envoy Docker image to run.
	// +kubebuilder:default="registry.gitlab.com/acnodal/envoy-for-egw:latest"
	EnvoyImage string `json:"envoy-image,omitempty"`

	// EnvoyTemplate is the template that will be used to configure
	// Envoy for the load balancers that belong to this ServiceGroup.
	EnvoyTemplate marin3r.EnvoyConfigSpec `json:"envoy-template"`
}

// ServiceGroupStatus defines the observed state of ServiceGroup
type ServiceGroupStatus struct {
	// ProxySnapshotVersions is a map of current Envoy proxy
	// configuration snapshot versions for the LBs that belong to this
	// ServiceGroup. Each increments every time the snapshot changes. We
	// store them here because they need to survive pod restarts.
	// +kubebuilder:default={}
	ProxySnapshotVersions map[string]int `json:"proxy-snapshot-versions"`

	// CurrentServiceID stores the most-recently-allocated GUE Service
	// ID for services in this group. See the comments on CurrentGroupID
	// in the EGW CR for notes on how to use this field.
	CurrentServiceID uint16 `json:"current-service-id"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=sg;sgs
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
