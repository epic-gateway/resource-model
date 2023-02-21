package v1

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink/nl"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func init() {
	SchemeBuilder.Register(&GWProxy{}, &GWProxyList{})
}

const (
	// gwproxyAgentFinalizerName is the name of the finalizer that
	// cleans up on each node when a GWProxy CR is deleted. It's used by
	// the AgentFinalizerName function below so this const isn't
	// exported.
	gwproxyAgentFinalizerName string = "gwp.node-agent.epic.acnodal.io"
)

// Important: Run "make" to regenerate code after modifying this file

// Important: We use the GWProxySpec to generate patches (e.g.,
// build an object and then marshal it to json) so *every* field
// except bool values must be "omitempty" or the resulting patch might
// wipe out some existing fields in the CR when the patch is applied.

// GWProxySpec defines the desired state of GWProxy
type GWProxySpec struct {
	// ClientRef points back to the client-side object that corresponds
	// to this one.
	ClientRef ClientRef `json:"clientRef,omitempty"`

	// DisplayName is the publicly-visible load balancer name (i.e.,
	// what the user specified). The CR name has a prefix and suffix to
	// disambiguate and we don't need to show those to the user.
	DisplayName string `json:"display-name,omitempty"`

	// PublicAddress is the publicly-visible IP address for this LB.
	PublicAddress string `json:"public-address,omitempty"`

	// AltAddress is a secondary IP address for this LB. When the
	// PublicAddress is an IPV6 address then we also need to attach an
	// IPV4 address to the proxy pod to enable IPV4 traffic in and out
	// of the pod.
	AltAddress string `json:"alt-address,omitempty"`

	// PublicPorts is the set of ports on which this LB will listen.
	PublicPorts []corev1.ServicePort `json:"public-ports,omitempty"`

	// TunnelKey authenticates clients with the EPIC. It must be a
	// base64-encoded 128-bit value. If not present, this will be filled
	// in by the defaulting webhook.
	TunnelKey string `json:"tunnel-key,omitempty"`

	// EnvoyTemplate is the template that will be used to configure
	// Envoy for the load balancers that belong to this LBServiceGroup. It
	// can be provided by the user, but if not it will be copied from
	// the owning LBServiceGroup.
	EnvoyTemplate *marin3r.EnvoyConfigSpec `json:"envoy-template,omitempty"`

	// EnvoyReplicaCount determines the number of Envoy proxy pod
	// replicas that will be launched for this GWProxy. If it's not
	// set then the controller will copy the value from the owning
	// LBServiceGroup.
	EnvoyReplicaCount *int32 `json:"envoy-replica-count,omitempty"`

	// GUETunnelEndpoints is a map of maps. The outer map is from client
	// node addresses to public GUE tunnel endpoints on the EPIC. The
	// map key is a client node address and must be one of the node
	// addresses in the Spec Endpoints slice. The value is a map
	// containing TunnelEndpoints that describes the public IPs and
	// ports to which the client can send tunnel ping packets. The key
	// is the IP address of the EPIC node and the value is a
	// TunnelEndpoint.
	GUETunnelMaps map[string]EPICEndpointMap `json:"gue-tunnel-endpoints,omitempty"`

	// ProxyInterfaces contains information about the Envoy proxy pods'
	// network interfaces. The map key is the proxy pod name. It's
	// filled in by the python setup-network daemon and used by the
	// gwproxy controller.
	ProxyInterfaces map[string]ProxyInterfaceInfo `json:"proxy-if-info,omitempty"`

	// Endpoints is a slice of DNS entries that external-dns will push
	// to our DNS provider. For now it typically holds one entry which
	// is generated from a template.
	Endpoints []*Endpoint `json:"endpoints,omitempty"`

	// Gateway is the client-side gatewayv1a2.GatewaySpec that
	// corresponds to this GWP.
	Gateway gatewayv1a2.GatewaySpec `json:"gateway,omitempty"`
}

// GWProxyStatus defines the observed state of GWProxy
type GWProxyStatus struct {
	// The generation observed by the external-dns controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=gwp;gwps
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.clusterID",name=Client Cluster,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.namespace",name=Client NS,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.name",name=Client Name,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.public-address",name=Public Address,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.epic\\.acnodal\\.io/owning-lbservicegroup",name=Service Group,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.epic\\.acnodal\\.io/owning-serviceprefix",name=Service Prefix,type=string

// GWProxy is the Schema for the gwproxies API
type GWProxy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GWProxySpec   `json:"spec"`
	Status GWProxyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GWProxyList contains a list of GWProxy
type GWProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GWProxy `json:"items"`
}

// NamespacedName returns a NamespacedName object filled in with this
// Object's name info.
func (proxy *GWProxy) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: proxy.Namespace, Name: proxy.Name}
}

// GWProxyName returns the name that we use in the GWProxy
// custom resource. This is a combo of the ServicePrefix name and the
// "raw" load balancer name. We need to smash them together because
// one customer might have two or more LBs with the same name, but
// belonging to different service prefixes, and a customer's LBs all
// live in the same k8s namespace so we need to make the service group
// name into an ersatz namespace.
func GWProxyName(sgName string, lbName string, canBeShared bool) (name string) {

	// The group name is a sorta-kinda "namespace" because two services
	// with the same name in the same group will be shared but two
	// services with the same name in different groups will be
	// independent. Since the gwproxy CRs for both services live in
	// the same k8s namespace, to make this work we prepend the group
	// name to the service name. This way two services with the same
	// name won't collide if they belong to different groups.
	name = sgName + "-" + lbName

	// If this LB can not be shared then give it a random readable name
	// so it doesn't collide with other LBs in the group that might be
	// created by other services with the same name. If the LB can be
	// shared then leave its name alone so other PureLB services with
	// the same name can find it. When they try to create their services
	// they'll get a 409 Conflict response that points them at this
	// service.
	if !canBeShared {
		raw := make([]byte, 8, 8)
		_, _ = rand.Read(raw)
		name += "-" + hex.EncodeToString(raw)
	}

	return name
}

// AgentFinalizerName returns the finalizer name for the given
// nodeName.
func (lb *GWProxy) AgentFinalizerName(nodeName string) string {
	return nodeName + "." + gwproxyAgentFinalizerName
}

// AddDNSEndpoint adds an external-dns Endpoint struct to the LB's
// Spec.Endpoints. The Endpoint is based on the LBSG's template and
// the DNS name is generated from a template. Parameters provided to
// the template: .LBName, .LBSGName, .ClusterServiceName,
// .ClusterServiceNS, .Account, .IPAddress (filtered to work in a DNS
// name).
func (proxy *GWProxy) AddDNSEndpoint(lbsg LBServiceGroup) error {
	var (
		tmpl *template.Template
		err  error
		doc  bytes.Buffer
	)

	// Execute the template from the LBSG to format the LB's DNS name.
	if tmpl, err = template.New("DNSName").Parse(lbsg.Spec.EndpointTemplate.DNSName); err != nil {
		return err
	}
	params := struct {
		LBName             string
		LBSGName           string
		ClusterServiceName string
		ClusterServiceNS   string
		Account            string
		IPAddress          string
	}{
		proxy.Name,
		lbsg.Name,
		proxy.Spec.DisplayName,
		proxy.Spec.ClientRef.Namespace,
		proxy.Labels[OwningAccountLabel],
		rfc1123Cleaner.Replace(proxy.Spec.PublicAddress),
	}
	err = tmpl.Execute(&doc, params)
	if err != nil {
		return err
	}

	// Assume that the IP address is a V4 so its address record type is
	// "A".
	addrType := "A"
	// If the addr is actually a V6 then the corresponding DNS address
	// record is "AAAA".
	if addrFamily(net.ParseIP(proxy.Spec.PublicAddress)) == nl.FAMILY_V6 {
		addrType = "AAAA"
	}

	// Fill in the Endpoint
	ep := Endpoint{}
	lbsg.Spec.EndpointTemplate.DeepCopyInto(&ep)
	ep.DNSName = doc.String()
	ep.Targets = append(ep.Targets, proxy.Spec.PublicAddress)
	if ep.RecordType == "" {
		ep.RecordType = addrType
	}

	// Add the Endpoint to the list
	proxy.Spec.Endpoints = append(proxy.Spec.Endpoints, &ep)

	return nil
}

// getLBServiceGroup gets this LB's owning LBServiceGroup.
func (proxy *GWProxy) getLBServiceGroup(ctx context.Context, cl client.Client) (*LBServiceGroup, error) {
	if proxy.Labels[OwningLBServiceGroupLabel] == "" {
		return nil, fmt.Errorf("LB has no owning service group label")
	}
	sgName := types.NamespacedName{Namespace: proxy.Namespace, Name: proxy.Labels[OwningLBServiceGroupLabel]}
	sg := &LBServiceGroup{}
	return sg, cl.Get(ctx, sgName, sg)
}

// GetChildRoutes lists the routes that reference this proxy and that
// are active, i.e., not in the process of being deleted.
func (proxy *GWProxy) GetChildRoutes(ctx context.Context, cl client.Client, l logr.Logger) ([]GWRoute, error) {
	list := GWRouteList{}
	err := cl.List(ctx, &list, &client.ListOptions{Namespace: proxy.Namespace})

	children := []GWRoute{}
	// build a new list with only routes that reference this proxy
	for _, route := range list.Items {
		for _, ref := range route.Parents() {
			if string(ref.Name) == proxy.Name && route.ObjectMeta.DeletionTimestamp.IsZero() {
				children = append(children, route)
			}
		}
	}

	return children, err
}

// RemoveProxyInfo removes podName's info from lbName's ProxyInterfaces
// map.
func RemoveProxyInfo(ctx context.Context, cl client.Client, namespace string, proxyName string, podName string) error {
	var proxy GWProxy

	key := client.ObjectKey{Namespace: namespace, Name: proxyName}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, &proxy); err != nil {
			return client.IgnoreNotFound(err)
		}

		if podInfo, hasProxy := proxy.Spec.ProxyInterfaces[podName]; hasProxy {
			// Remove this pod's entry from the ProxyInterfaces map
			delete(proxy.Spec.ProxyInterfaces, podName)

			// Find out if any remaining Envoy pods are running on the same
			// node as the one we've deleted.
			hasProxys := false
			for _, nodeInfo := range proxy.Spec.ProxyInterfaces {
				if podInfo.EPICNodeAddress == nodeInfo.EPICNodeAddress {
					hasProxys = true
				}
			}

			// If this node has no more Envoy pods then remove this node's
			// entry from any of the EPICEndpointMaps that have it.
			if !hasProxys {
				for _, epTunnelMap := range proxy.Spec.GUETunnelMaps {
					delete(epTunnelMap.EPICEndpoints, podInfo.EPICNodeAddress)
				}
			}

			// Try to update
			return cl.Update(ctx, &proxy)
		}

		return nil
	})
}

// ActiveProxyEndpoints is a kludge to let us use old code that
// expects RemoteEndpoints with our new GWRoute/GWEndpointSlice
// models. It iterates through the GWRoutes and GWEndpointSlices and
// creates fake RemoteEndpoints that belong to the proxy and that are
// active, i.e., not in the process of being deleted.
func (proxy *GWProxy) ActiveProxyEndpoints(ctx context.Context, cl client.Client) ([]RemoteEndpoint, error) {
	activeEPs := []RemoteEndpoint{}
	listOps := client.ListOptions{Namespace: proxy.Namespace}
	gwName := types.NamespacedName{Namespace: proxy.Namespace, Name: proxy.Name}

	// List all of the routes in this NS. We'll check later if any
	// reference this proxy.
	routes := GWRouteList{}
	err := cl.List(ctx, &routes, &listOps)
	if err != nil {
		return activeEPs, err
	}

	// List all of the slices in this NS. We'll check later if any
	// reference the same cluster that any of the routes do.
	slices := GWEndpointSliceList{}
	err = cl.List(ctx, &slices, &listOps)
	if err != nil {
		return activeEPs, err
	}

	for _, route := range routes.Items {
		if route.ObjectMeta.DeletionTimestamp.IsZero() && route.isChildOf(gwName) {
			for _, rule := range route.Spec.HTTP.Rules {
				for _, ref := range rule.BackendRefs {

					// clusterName is the "glue" that binds GWRoute and GWEndpointSlice together
					clusterName := string(ref.Name)

					for _, slice := range slices.Items {
						if slice.Spec.ParentRef.UID == clusterName && slice.ObjectMeta.DeletionTimestamp.IsZero() {
							// FIXME: call slice.ToEndpoints() here
							for _, endpoint := range slice.Spec.Endpoints {
								for _, address := range endpoint.Addresses {
									// 'sup dawg? I heard you like loops so I put some
									// loops in your loops
									activeEPs = append(activeEPs, RemoteEndpoint{
										Spec: RemoteEndpointSpec{
											Cluster:     clusterName,
											Address:     address,
											NodeAddress: slice.Spec.NodeAddresses[*endpoint.NodeName],
											Port: v1.EndpointPort{
												Port:     *slice.Spec.Ports[0].Port,
												Protocol: *slice.Spec.Ports[0].Protocol,
											},
										},
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return activeEPs, err
}

// Nudge "nudges" the GWProxy, i.e., causes its reconciler to fire, by
// adding a random annotation.
func (proxy *GWProxy) Nudge(ctx context.Context, cl client.Client, l logr.Logger) error {
	var (
		err        error
		patch      []map[string]interface{}
		patchBytes []byte
		raw        []byte = make([]byte, 8, 8)
	)

	_, _ = rand.Read(raw)

	// Prepare the patch with the new annotation.
	if proxy.Annotations == nil {
		// If this is the first annotation then we need to wrap it in a
		// JSON object
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": map[string]string{"nudge": hex.EncodeToString(raw)},
		}}
	} else {
		// If there are other annotations then we can just add this one
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations/nudge",
			"value": hex.EncodeToString(raw),
		}}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	if err := cl.Patch(ctx, proxy, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		l.Error(err, "patching", "proxy", proxy)
		return err
	}
	l.V(1).Info("patched", "proxy", proxy)

	return nil
}

// RemovePodInfo removes podName's info from name's ProxyInterfaces
// map.
func RemovePodInfo(ctx context.Context, cl client.Client, ns string, name string, podName string) error {
	var p GWProxy

	key := client.ObjectKey{Namespace: ns, Name: name}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, &p); err != nil {
			return err
		}

		if podInfo, hasPod := p.Spec.ProxyInterfaces[podName]; hasPod {
			// Remove this pod's entry from the ProxyInterfaces map
			delete(p.Spec.ProxyInterfaces, podName)

			// Find out if any remaining Envoy pods are running on the same
			// node as the one we've deleted.
			hasPods := false
			for _, nodeInfo := range p.Spec.ProxyInterfaces {
				if podInfo.EPICNodeAddress == nodeInfo.EPICNodeAddress {
					hasPods = true
				}
			}

			// If this node has no more Envoy pods then remove this node's
			// entry from any of the EPICEndpointMaps that have it.
			if !hasPods {
				for _, epTunnelMap := range p.Spec.GUETunnelMaps {
					delete(epTunnelMap.EPICEndpoints, podInfo.EPICNodeAddress)
				}
			}

			// Try to update
			return cl.Update(ctx, &p)
		}

		return nil
	})
}
