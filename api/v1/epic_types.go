package v1

import (
	"context"
	"net"
	"os"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// +kubebuilder:default="quay.io/epic-gateway/eds-server:latest"
	EDSImage string `json:"eds-image"`

	// EnvoyImage is the default Envoy Docker image name. This value can
	// be overridden by the EnvoyImage field in the lbservicegroup which
	// allows customers to specify their own Envoy images.
	EnvoyImage string `json:"envoy-image"`

	// XDSImage, if set, specifies the Marin3r discovery service image
	// to run.  If not set, the default will be the image specified in
	// the marin3r deployment manifest.
	XDSImage *string `json:"xds-image,omitempty"`

	// ServiceCIDR is the pool from which internal service addresses are
	// allocated. In microk8s it's hard-coded and passed on the
	// kubeapiserver command line (see
	// epicmgr-resources/default-args/kube-apiserver). We need a way to
	// discover this value so we can configure routes in the Envoy pod.
	// The installer will set this value when it installs the epic
	// singleton custom resource.
	ServiceCIDR string `json:"service-cidr"`

	// NodeBase is the "base" configuration for all nodes in the
	// cluster.
	NodeBase Node `json:"base"`
}

// EPICEndpointMap contains a map of the EPIC endpoints that connect
// to one PureLB endpoint, keyed by Node IP address.
type EPICEndpointMap struct {
	EPICEndpoints map[string]GUETunnelEndpoint `json:"epic-endpoints,omitempty"`
}

// GUETunnelEndpoint is an Endpoint on the EPIC.
type GUETunnelEndpoint struct {
	// Address is the IP address on the EPIC for this endpoint.
	Address string `json:"epic-address,omitempty"`

	// Port is the port on which this endpoint listens.
	// +kubebuilder:default={"port":6080,"protocol":"UDP","appProtocol":"gue"}
	Port corev1.EndpointPort `json:"epic-port,omitempty"`

	// TunnelID is used to route traffic to the correct tunnel.
	TunnelID uint32 `json:"tunnel-id,omitempty"`
}

// ProxyInterfaceInfo contains information about the Envoy proxy pod's
// network interfaces.
type ProxyInterfaceInfo struct {
	// EPICNodeAddress is the IP address of the EPIC node that hosts
	// this proxy pod.
	EPICNodeAddress string `json:"epic-node-address,omitempty"`

	// Port is the port on which this endpoint listens.
	// +kubebuilder:default={"port":6080,"protocol":"UDP","appProtocol":"gue"}
	Port corev1.EndpointPort `json:"epic-port,omitempty"`

	// Index is the ifindex of the Envoy proxy pod's veth interface on
	// the CRI side of this service's proxy pod. In other words, it's
	// the end of the veth that's inside the container (i.e., on the
	// other side from the end that's attached to the multus bridge).
	Index int `json:"index,omitempty"`

	// Name is the name of the interface whose index is ProxyIfindex.
	Name string `json:"name,omitempty"`
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

// IngressIPAddr returns the first IP address from the first interface in
// this node's IngressNICs list. If error is non-nil then something
// has gone wrong.
func (node *Node) IngressIPAddr() (string, error) {
	link, err := netlink.LinkByName(node.IngressNICs[0])
	if err != nil {
		return "", err
	}
	addrs, err := netlink.AddrList(link, addrFamily(net.ParseIP(os.Getenv("EPIC_HOST"))))
	if err != nil {
		return "", err
	}
	addr := addrs[0].IP.String()
	return addr, nil
}

// addrFamily returns whether lbIP is an IPV4 or IPV6 address.  The
// return value will be nl.FAMILY_V6 if the address is an IPV6
// address, nl.FAMILY_V4 if it's IPV4, or 0 if the family can't be
// determined.
func addrFamily(lbIP net.IP) (lbIPFamily int) {
	if nil != lbIP.To16() {
		lbIPFamily = nl.FAMILY_V6
	}

	if nil != lbIP.To4() {
		lbIPFamily = nl.FAMILY_V4
	}

	return
}

// AllocateTunnelID allocates a tunnel ID from the EPIC singleton. If
// this call succeeds (i.e., error is nil) then the returned ID will
// be unique.
func AllocateTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {
	return tunnelID, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var epic EPIC

		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err = cl.Get(ctx, client.ObjectKey{Namespace: ConfigNamespace, Name: ConfigName}, &epic); err != nil {
			return err
		}

		if epic.Status.CurrentTunnelID == 0 {
			l.Info("adjusting tunnelID")
			epic.Status.CurrentTunnelID = 1
		}

		tunnelID = epic.Status.CurrentTunnelID
		epic.Status.CurrentTunnelID++

		// Try to update
		return cl.Status().Update(ctx, &epic)
	})
}
