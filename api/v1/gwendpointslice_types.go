package v1

import (
	"context"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	SchemeBuilder.Register(&GWEndpointSlice{}, &GWEndpointSliceList{})
}

// GWEndpointSliceSpec is a container for the EndpointSlice object in
// the client cluster. It also contains the info needed to find the
// object on the client side.
type GWEndpointSliceSpec struct {
	// ClientRef points back to the client-side object that corresponds
	// to this one.
	ClientRef ClientRef `json:"clientRef,omitempty"`

	// ParentRef points to the client-side service that owns this slice.
	ParentRef ClientRef `json:"parentRef,omitempty"`

	// Slice holds the client-side EndpointSlice contents.
	discoveryv1.EndpointSlice `json:",inline"`

	// Map of node addresses. Key is node name, value is IP address
	// represented as string.
	NodeAddresses map[string]string `json:"nodeAddresses"`
}

// GWEndpointSliceStatus defines the observed state of GWEndpointSlice.
type GWEndpointSliceStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=gwes;gwess,path=gwendpointslices,singular=gwendpointslice
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".spec.clientRef.clusterID",name=Client Cluster,type=string
//+kubebuilder:printcolumn:JSONPath=".spec.clientRef.namespace",name=Client NS,type=string
//+kubebuilder:printcolumn:JSONPath=".spec.clientRef.name",name=Client Name,type=string

// GWEndpointSlice corresponds to an EndpointSlice object in a client
// cluster. It provides data for Envoy and to set up the GUE tunnels.
type GWEndpointSlice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GWEndpointSliceSpec   `json:"spec,omitempty"`
	Status GWEndpointSliceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GWEndpointSliceList contains a list of GWEndpointSlice
type GWEndpointSliceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GWEndpointSlice `json:"items"`
}

// ReferencingProxies returns an array of the GWProxies that link to
// this slice via GWRoutes.
func (slice *GWEndpointSlice) ReferencingProxies(ctx context.Context, cl client.Client) ([]*GWProxy, error) {
	refs := []*GWProxy{}

	listOps := client.ListOptions{Namespace: slice.Namespace}
	routes := GWRouteList{}
	err := cl.List(ctx, &routes, &listOps)
	if err != nil {
		return refs, err
	}

	for _, route := range routes.Items {
		for _, rule := range route.Spec.HTTP.Rules {
			for _, ref := range rule.BackendRefs {
				backendName := string(ref.Name)
				if backendName == slice.Spec.ParentRef.UID {
					for _, parent := range route.Spec.HTTP.ParentRefs {
						proxy := GWProxy{}
						proxyName := types.NamespacedName{Namespace: slice.Namespace, Name: string(parent.Name)}
						if err := cl.Get(ctx, proxyName, &proxy); err != nil {
							// If there's an error other than "not found" then bail
							// out and report it. If we can't find the Proxy that's
							// probably because it was deleted which isn't an error.
							if client.IgnoreNotFound(err) != nil {
								return refs, err
							}
						} else {
							refs = append(refs, &proxy)
						}
					}
				}
			}
		}
	}

	return refs, nil
}

// ToEndpoints represents this slice as an array of our LB
// RemoteEndpoints.
func (slice *GWEndpointSlice) ToEndpoints() []RemoteEndpoint {
	activeEPs := []RemoteEndpoint{}
	for _, endpoint := range slice.Spec.Endpoints {
		for _, address := range endpoint.Addresses {
			activeEPs = append(activeEPs, RemoteEndpoint{
				Spec: RemoteEndpointSpec{
					Cluster:     slice.Spec.ParentRef.UID,
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

	return activeEPs
}
