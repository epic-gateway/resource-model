package v1

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func init() {
	SchemeBuilder.Register(&GWRoute{}, &GWRouteList{})
}

// GWRouteSpec is our wrapper for the various types of Gateway
// RouteSpecs.
type GWRouteSpec struct {
	// ClientRef points back to the client-side object that corresponds
	// to this one.
	ClientRef ClientRef                  `json:"clientRef,omitempty"`
	HTTP      *gatewayv1a2.HTTPRouteSpec `json:"http,omitempty"`
	TCP       *gatewayv1a2.TCPRouteSpec  `json:"tcp,omitempty"`
}

// GWRouteStatus defines the observed state of GWRoute
type GWRouteStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=gwr;gwrs
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.clusterID",name=Client Cluster,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.namespace",name=Client NS,type=string
// +kubebuilder:printcolumn:JSONPath=".spec.clientRef.name",name=Client Name,type=string

// GWRoute is the Schema for the gwroutes API
type GWRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GWRouteSpec   `json:"spec,omitempty"`
	Status GWRouteStatus `json:"status,omitempty"`
}

// GWRouteName makes a name for this route that will be unique within
// this customer's namespace. It should also be somewhat
// human-readable which will hopefully help with debugging.
func (gwr *GWRoute) GWRouteName() string {
	raw := make([]byte, 8, 8)
	_, _ = rand.Read(raw)
	return fmt.Sprintf("%s-%s", gwr.Name, hex.EncodeToString(raw))
}

//+kubebuilder:object:root=true

// GWRouteList contains a list of GWRoute
type GWRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GWRoute `json:"items"`
}

// Parents returns a slice containing this GWR's ParentReferences.
func (gwr *GWRoute) Parents() (parents []gatewayv1a2.ParentReference) {
	if gwr.Spec.HTTP != nil {
		parents = append(parents, gwr.Spec.HTTP.ParentRefs...)
	}
	if gwr.Spec.TCP != nil {
		parents = append(parents, gwr.Spec.TCP.ParentRefs...)
	}

	return
}

// isChildOf indicates whether this GWRoute has a ParentRef that
// references gateway.
func (gwr *GWRoute) isChildOf(gateway types.NamespacedName) bool {
	for _, ref := range gwr.Parents() {
		if string(ref.Name) == gateway.Name {
			return true
		}
	}

	return false
}
