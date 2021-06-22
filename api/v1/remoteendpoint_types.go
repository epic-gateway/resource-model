package v1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// RemoteEndpointFinalizerName is the name of the finalizer that
	// does the per-cluster cleanup when a RemoteEndpoint CR is deleted.
	RemoteEndpointFinalizerName string = "remoteendpoint-finalizer.controller-manager.acnodal.io"
	// RemoteEndpointAgentFinalizerName is the name of the finalizer
	// that does the per-node cleanup when a RemoteEndpoint CR is
	// deleted.
	RemoteEndpointAgentFinalizerName string = "remoteendpoint-agent-finalizer.controller-manager.acnodal.io"
)

var (
	rfc1123Cleaner = strings.NewReplacer(".", "-", ":", "-")
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

// RemoteEndpointSpec defines the desired state of RemoteEndpoint. It
// represents one pod endpoint on a customer cluster.
type RemoteEndpointSpec struct {
	// Cluster is the cluster-id of the cluster to which this rep
	// belongs.
	Cluster string `json:"cluster"`

	// Address is the IP address for this endpoint.
	Address string `json:"address"`

	// NodeAddress is the IP address of the node on which this endpoint
	// is running. We use it to set up a GUE tunnel from the EPIC to the
	// node. If it is not set then this endpoint will be ad-hoc, i.e.,
	// it won't use GUE.
	NodeAddress string `json:"node-address,omitempty"`

	// Port is the port on which this endpoint listens.
	Port corev1.EndpointPort `json:"port"`
}

// RemoteEndpointStatus defines the observed state of RemoteEndpoint
type RemoteEndpointStatus struct {
	// The ProxyIfindex in the LoadBalancer Status is canonical but we
	// cache it here so we can cleanup the PFC service without having to
	// lookup the LB since it might have been deleted.
	ProxyIfindex int `json:"proxy-ifindex,omitempty"`

	// GroupID is a cache of the owning Account's GroupID. The ID in the
	// Account Spec is canonical but we cache it here so we can cleanup
	// the PFC service without having to lookup the LB since it might
	// have been deleted.
	GroupID uint16 `json:"group-id,omitempty"`

	// ServiceID is a cache of the owning LoadBalancer's ServiceID. The
	// ID in the LoadBalancer Spec is canonical but we cache it here so
	// we can cleanup the PFC service without having to lookup the LB
	// since it might have been deleted.
	ServiceID uint16 `json:"service-id,omitempty"`

	// The TunnelID in the LoadBalancer Status is canonical but we cache
	// it here so we can cleanup the PFC service without having to
	// lookup the LB since it might have been deleted.
	TunnelID uint32 `json:"tunnel-id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=rep;reps
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.owning-loadbalancer",name=Load Balancer,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.address",name=Address,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.node-address",name=Node Address,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.port.port",name=Port,type=integer
// +kubebuilder:printcolumn:JSONPath=".spec.port.protocol",name=Protocol,type=string

// RemoteEndpoint represents a service endpoint on a remote customer
// cluster.
type RemoteEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteEndpointSpec   `json:"spec,omitempty"`
	Status RemoteEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteEndpointList contains a list of RemoteEndpoint
type RemoteEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteEndpoint `json:"items"`
}

// RemoteEndpointName makes a name for this rep that will be unique
// within this customer's namespace. It should also be somewhat
// human-readable which will hopefully help with debugging.
func RemoteEndpointName(address net.IP, port int32, protocol v1.Protocol) string {
	raw := make([]byte, 8, 8)
	_, _ = rand.Read(raw)
	return fmt.Sprintf("%s-%d-%s-%s", rfc1123Cleaner.Replace(address.String()), port, toLower(protocol), hex.EncodeToString(raw))
}

func toLower(protocol v1.Protocol) string {
	return strings.ToLower(string(protocol))
}

func init() {
	SchemeBuilder.Register(&RemoteEndpoint{}, &RemoteEndpointList{})
}
