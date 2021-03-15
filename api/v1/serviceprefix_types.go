package v1

import (
	"github.com/vishvananda/netlink"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Important: Run "make" to regenerate code after modifying this file

// ServicePrefixSpec defines the desired state of ServicePrefix
type ServicePrefixSpec struct {
	// Subnet is the subnet in which all of the pool addresses live. It
	// must be in CIDR notation, e.g., "192.168.77.0/24".
	Subnet string `json:"subnet"`
	// Pool is the set of addresses to be allocated. It must be in
	// from-to notation, e.g., "192.168.77.2-192.168.77.8".
	Pool string `json:"pool"`
	// Gateway is the gateway address of this ServicePrefix. It should
	// be specified as an IP address alone, with no subnet.
	Gateway string `json:"gateway"`

	// +kubebuilder:default=default
	Aggregation string `json:"aggregation,omitempty"`
	// +kubebuilder:default=multus0
	MultusBridge string `json:"multus-bridge,omitempty"`
}

// GatewayAddr returns this ServicePrefix's gateway in the form of a netlink.Addr.
func (sps *ServicePrefixSpec) GatewayAddr() *netlink.Addr {
	sn, err := netlink.ParseIPNet(sps.Subnet)
	if err != nil {
		return nil
	}

	// parse with a dummy /32 for now, we'll override later
	addr, err := netlink.ParseAddr(sps.Gateway + "/32")
	if err != nil {
		return nil
	}

	// the gateway is the address from the "gateway" field but with the
	// subnet mask from the "subnet" field
	addr.Mask = sn.Mask

	return addr
}

// ServicePrefixStatus defines the observed state of ServicePrefix
type ServicePrefixStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=sp;sps
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
