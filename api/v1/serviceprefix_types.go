package v1

import (
	"context"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// be specified as an IP address alone, with no subnet. If no
	// Gateway is provided the multus0 bridge won't have an IP address.
	Gateway *string `json:"gateway,omitempty"`

	// +kubebuilder:default=default
	Aggregation string `json:"aggregation,omitempty"`
	// +kubebuilder:default=multus0
	MultusBridge string `json:"multus-bridge,omitempty"`
}

// SubnetIPNet returns this ServicePrefix's subnet in the form of a net.IPNet.
func (sps *ServicePrefixSpec) SubnetIPNet() (*net.IPNet, error) {
	return netlink.ParseIPNet(sps.Subnet)
}

// GatewayAddr returns this ServicePrefix's gateway in the form of a netlink.Addr.
func (sps *ServicePrefixSpec) GatewayAddr() *netlink.Addr {
	if sps.Gateway == nil {
		return nil
	}

	sn, err := netlink.ParseIPNet(sps.Subnet)
	if err != nil {
		return nil
	}

	// parse with a dummy /32 for now, we'll override later
	addr, err := netlink.ParseAddr(*sps.Gateway + "/32")
	if err != nil {
		return nil
	}

	// the gateway is the address from the "gateway" field but with the
	// subnet mask from the "subnet" field
	addr.Mask = sn.Mask

	return addr
}

// aggregateRoute determines the aggregated IP network for lbIP, given
// this SP's subnet and aggregation. Aggregation determines whether to
// use default aggregation or an override. If aggregation is "default"
// then the mask from subnet will be used. Otherwise aggregation must
// be the netmask part of a CIDR address, e.g., "/32".
func (sps *ServicePrefixSpec) aggregateRoute(lbIP net.IP) (network net.IPNet, err error) {
	// Assume that the aggregation is "default"
	rawCIDR := sps.Subnet

	// If aggregation is not "default" then use it instead of the
	// default
	if sps.Aggregation != "default" {
		rawCIDR = fmt.Sprintf("%s%s", lbIP.String(), sps.Aggregation)
	}

	// Parse the CIDR into a net.IPNet to return to the caller
	_, lbIPNet, err := net.ParseCIDR(rawCIDR)
	if err != nil {
		return net.IPNet{}, err
	}
	return *lbIPNet, err
}

// ServicePrefixStatus defines the observed state of ServicePrefix
type ServicePrefixStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=sp;sps
// +kubebuilder:subresource:status

// ServicePrefix represents a pool of IP addresses. The EPIC web
// service will allocate addresses from the set of ServicePrefixes.
type ServicePrefix struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServicePrefixSpec   `json:"spec,omitempty"`
	Status ServicePrefixStatus `json:"status,omitempty"`
}

// AddMultusRoute adds a route to dest to this SP's multus bridge.
func (sp *ServicePrefix) AddMultusRoute(lbIP net.IP) error {
	var (
		dest net.IPNet
		link netlink.Link
		err  error
	)

	// Find our Multus bridge Link
	if link, err = netlink.LinkByName(sp.Spec.MultusBridge); err != nil {
		return err
	}

	// Aggregate the IP address
	if dest, err = sp.Spec.aggregateRoute(lbIP); err != nil {
		return err
	}

	// Add the route
	if err := netlink.RouteReplace(&netlink.Route{LinkIndex: link.Attrs().Index, Dst: &dest}); err != nil {
		return err
	}

	return nil
}

// RemoveMultusRoute removes the multus bridge route for lbIP only if
// no other proxy is using it. Because we can aggregate addresses, one
// route might attract traffic for more than one IP address. We don't
// want to remove a route until *all* of the IPs that depend on it are
// gone.
func (sp *ServicePrefix) RemoveMultusRoute(ctx context.Context, r client.Reader, l logr.Logger, proxyName string, lbIP net.IP) error {
	var (
		dest       net.IPNet
		gwps       GWProxyList
		routeInUse bool = false
		err        error
	)

	// Aggregate the IP address
	if dest, err = sp.Spec.aggregateRoute(lbIP); err != nil {
		return err
	}

	// List the set of proxies that belong to this SP
	listOps := client.ListOptions{
		Namespace:     "", // all namespaces (since SPs live in "epic")
		LabelSelector: labels.SelectorFromSet(map[string]string{OwningServicePrefixLabel: sp.Name}),
	}
	if err := r.List(ctx, &gwps, &listOps); err != nil {
		return err
	}

	// Check to see if any other proxies are using the same aggregated route
	for _, otherProxy := range gwps.Items {
		// Only check *other* proxies, not this one
		if otherProxy.Name == proxyName {
			continue
		}

		// Only check proxies that aren't being deleted
		if !otherProxy.ObjectMeta.DeletionTimestamp.IsZero() {
			continue
		}

		// If the aggregated route is the same (i.e., if another proxy is
		// using it) then mark the route as "in use"
		otherAddr := net.ParseIP(otherProxy.Spec.PublicAddress)
		if otherAddr == nil {
			continue
		}
		otherDest, err := sp.Spec.aggregateRoute(otherAddr)
		if err == nil && otherDest.String() == dest.String() {
			l.Info("route in use, not removing", "route", dest.String())
			routeInUse = true
			break
		}
	}

	// If the route is not in use by any other proxy then we can delete it
	if !routeInUse {
		l.Info("removing route", "route", dest.String())
		return netlink.RouteDel(&netlink.Route{Dst: &dest})
	}

	return nil
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
