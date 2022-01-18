package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GWClusterSpec defines the desired state of GWCluster
type GWClusterSpec struct {
	// ClientRef points back to the client-side object that corresponds
	// to this one.
	ClientRef ClientRef `json:"clientRef,omitempty"`

	// Foo is an example field of GWCluster. Edit gwcluster_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// GWClusterStatus defines the observed state of GWCluster
type GWClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.clusterID",name=Client Cluster,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.namespace",name=Client NS,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.name",name=Client Name,type=string

// GWCluster is the Schema for the gwclusters API
type GWCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GWClusterSpec   `json:"spec,omitempty"`
	Status GWClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GWClusterList contains a list of GWCluster
type GWClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GWCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GWCluster{}, &GWClusterList{})
}
