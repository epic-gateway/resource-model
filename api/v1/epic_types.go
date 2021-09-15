package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ProductName is the name of our product. It's EPIC!
	ProductName = "epic"

	// ConfigName is the name of the EPIC configuration singleton. Its
	// namespace is defined in namespaces.go.
	ConfigName = ProductName
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// Node is the config for one node.
type Node struct {
	// +kubebuilder:default={"enp1s0"}
	IngressNICs []string `json:"gue-ingress-nics,omitempty"`

	// +kubebuilder:default={"epic-port":{"port":6080,"protocol":"UDP","appProtocol":"gue"}}
	GUEEndpoint GUETunnelEndpoint `json:"gue-endpoint,omitempty"`
}

// EPICSpec defines the desired state of EPIC
type EPICSpec struct {
	// EDSImage is the name of the EPIC EDS control plane Docker image
	// to run.
	// +kubebuilder:default="registry.gitlab.com/acnodal/xds-operator:latest"
	EDSImage string `json:"eds-image"`

	// EnvoyImage is the default Envoy Docker image name. This value can
	// be overridden by the EnvoyImage field in the lbservicegroup which
	// allows customers to specify their own Envoy images.
	EnvoyImage string `json:"envoy-image"`

	// XDSImage, if set, specifies the Marin3r discovery service image
	// to run.  If not set, the default will be the image specified in
	// the marin3r deployment manifest.
	XDSImage *string `json:"xds-image,omitempty"`

	// ServiceCIDR is the pool from which internal service addresses
	// are allocated. In microk8s it's hard-coded and passed on the
	// kubeapiserver command line (see
	// epicmgr-resources/default-args/kube-apiserver). We need a way to
	// discover this value so we can configure routes in the Envoy pod.
	// The snap package will set this value when it installs the epic
	// singleton custom resource.
	ServiceCIDR string `json:"service-cidr"`

	// NodeBase is the "base" configuration for all nodes in the
	// cluster.
	NodeBase Node `json:"base"`
}

// EPICStatus defines the observed state of EPIC
type EPICStatus struct {
	// CurrentGroupID is no longer used.
	CurrentGroupID uint16 `json:"current-group-id"`

	// CurrentTunnelID stores the most-recently-allocated tunnel
	// ID. Clients should read the CR, calculate the next value and then
	// write that back using Update() and not Patch(). If the write
	// succeeds then they own that value. If not then they need to try
	// again.
	CurrentTunnelID uint32 `json:"current-tunnel-id"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EPIC is the Schema for the epics API
type EPIC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EPICSpec   `json:"spec,omitempty"`
	Status EPICStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EPICList contains a list of EPIC
type EPICList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EPIC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EPIC{}, &EPICList{})
}
