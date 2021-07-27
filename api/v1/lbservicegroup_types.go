package v1

import (
	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// LBServiceGroupSpec defines the desired state of LBServiceGroup
type LBServiceGroupSpec struct {
	// CanBeShared determines whether the LBs that belong to this SG can
	// be shared among multiple PureLB services.
	// +kubebuilder:default=false
	CanBeShared bool `json:"can-be-shared"`

	// EnvoyImage is the Envoy Docker image name. If this is not set
	// then the image specified in the EPIC config singleton EnvoyImage
	// field will be used.
	EnvoyImage *string `json:"envoy-image,omitempty"`

	// EnvoyReplicaCount determines the number of Envoy proxy pod
	// replicas that will be launched for each LoadBalancer. It can be
	// overridden by the LoadBalancer CR.
	// +kubebuilder:default=1
	EnvoyReplicaCount int32 `json:"envoy-replica-count"`

	// EnvoyTemplate is the template that will be used to configure
	// Envoy for the load balancers that belong to this LBServiceGroup.
	EnvoyTemplate marin3r.EnvoyConfigSpec `json:"envoy-template"`
}

// LBServiceGroupStatus defines the observed state of LBServiceGroup
type LBServiceGroupStatus struct {
	// ProxySnapshotVersions is a map of current Envoy proxy
	// configuration snapshot versions for the LBs that belong to this
	// LBServiceGroup. Each increments every time the snapshot changes. We
	// store them here because they need to survive pod restarts.
	// +kubebuilder:default={}
	ProxySnapshotVersions map[string]int `json:"proxy-snapshot-versions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=lbsg;lbsgs
// +kubebuilder:subresource:status

// LBServiceGroup is the Schema for the lbservicegroups API
type LBServiceGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LBServiceGroupSpec   `json:"spec,omitempty"`
	Status LBServiceGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LBServiceGroupList contains a list of LBServiceGroup
type LBServiceGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LBServiceGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LBServiceGroup{}, &LBServiceGroupList{})
}
