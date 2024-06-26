package v1

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// loadbalancerAgentFinalizerName is the name of the finalizer that
	// cleans up on each node when a LoadBalancer CR is deleted. It's
	// used by the AgentFinalizerName function below so this const isn't
	// exported.
	loadbalancerAgentFinalizerName string = "lb.controller-manager.acnodal.io/finalizer"
)

// Important: Run "make" to regenerate code after modifying this file

// Important: We use the LoadBalancerSpec to generate patches (e.g.,
// build an object and then marshal it to json) so *every* field
// except bool values must be "omitempty" or the resulting patch might
// wipe out some existing fields in the CR when the patch is applied.

// LoadBalancerSpec defines the desired state of LoadBalancer
type LoadBalancerSpec struct {
	// DisplayName is the publicly-visible load balancer name (i.e.,
	// what the user specified). The CR name has a prefix and suffix to
	// disambiguate and we don't need to show those to the user.
	DisplayName string `json:"display-name,omitempty"`

	// PublicAddress is the publicly-visible IP address for this LB.
	PublicAddress string `json:"public-address,omitempty"`

	// PublicPorts is the set of ports on which this LB will listen.
	PublicPorts []corev1.ServicePort `json:"public-ports,omitempty"`

	// EnvoyTemplate is the template that will be used to configure
	// Envoy for the load balancers that belong to this LBServiceGroup. It
	// can be provided by the user, but if not it will be copied from
	// the owning LBServiceGroup.
	EnvoyTemplate *marin3r.EnvoyConfigSpec `json:"envoy-template,omitempty"`

	// EnvoyReplicaCount determines the number of Envoy proxy pod
	// replicas that will be launched for this LoadBalancer. If it's not
	// set then the controller will copy the value from the owning
	// LBServiceGroup.
	EnvoyReplicaCount *int32 `json:"envoy-replica-count,omitempty"`

	// UpstreamClusters is a list of the names of the upstream clusters
	// (currently only PureLB but maybe other types in the future) that
	// this LB serves. The string must be the cluster-id that is passed
	// in the announce and withdraw web service methods.
	UpstreamClusters []string `json:"upstream-clusters,omitempty"`

	// TrueIngress indicates that this LB will use TrueIngress to talk
	// to its upstream cluster endpoints. The default is true since that
	// will likely be the most common case.
	// +kubebuilder:default=true
	TrueIngress bool `json:"true-ingress"`

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
	// loadbalancer controller.
	ProxyInterfaces map[string]ProxyInterfaceInfo `json:"proxy-if-info,omitempty"`

	Endpoints []*Endpoint `json:"endpoints,omitempty"`
}

// LoadBalancerStatus defines the observed state of LoadBalancer
type LoadBalancerStatus struct {
	// The generation observed by the external-dns controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=lb;lbs
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".spec.public-address",name=Public Address,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.epic\\.acnodal\\.io/owning-lbservicegroup",name=Service Group,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.labels.epic\\.acnodal\\.io/owning-serviceprefix",name=Service Prefix,type=string

// LoadBalancer is the Schema for the loadbalancers API
type LoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoadBalancerSpec   `json:"spec"`
	Status LoadBalancerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoadBalancerList contains a list of LoadBalancer
type LoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancer `json:"items"`
}

// NamespacedName returns a NamespacedName object filled in with this
// Object's name info.
func (lb *LoadBalancer) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: lb.Namespace, Name: lb.Name}
}

// AddUpstream adds "clusterId" as an upstream cluster to this
// LB. Returns an error if the LB already contained the cluster, nil
// if it didn't.
func (lb *LoadBalancer) AddUpstream(clusterID string) error {
	if lb.ContainsUpstream(clusterID) {
		return fmt.Errorf("LB %s already contains upstream %s", lb.NamespacedName(), clusterID)
	}
	lb.Spec.UpstreamClusters = append(lb.Spec.UpstreamClusters, clusterID)
	return nil
}

// ContainsUpstream indicates whether "contains" is already registered
// as an upstream cluster. Returns true if it is, false if it isn't.
func (lb *LoadBalancer) ContainsUpstream(contains string) bool {
	for _, upstream := range lb.Spec.UpstreamClusters {
		if contains == upstream {
			return true
		}
	}

	return false
}

// RemoveUpstream removes "clusterId" as an upstream cluster from this
// LB. Returns nil if the LB already contained the cluster, an error
// if it didn't.
func (lb *LoadBalancer) RemoveUpstream(clusterID string) error {
	for i, upstream := range lb.Spec.UpstreamClusters {
		if clusterID == upstream {
			// https://stackoverflow.com/a/37335777/5967960
			c := lb.Spec.UpstreamClusters
			c[i] = c[len(c)-1]
			lb.Spec.UpstreamClusters = c[:len(c)-1]
			return nil
		}
	}

	return fmt.Errorf("Upstream cluster \"%s\" not found in LoadBalancer \"%s\"", clusterID, lb.Name)
}

// LoadBalancerName returns the name that we use in the LoadBalancer
// custom resource. This is a combo of the ServicePrefix name and the
// "raw" load balancer name. We need to smash them together because
// one customer might have two or more LBs with the same name, but
// belonging to different service prefixes, and a customer's LBs all
// live in the same k8s namespace so we need to make the service group
// name into an ersatz namespace.
func LoadBalancerName(sgName string, lbName string, canBeShared bool) (name string) {

	// The group name is a sorta-kinda "namespace" because two services
	// with the same name in the same group will be shared but two
	// services with the same name in different groups will be
	// independent. Since the loadbalancer CRs for both services live in
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
func (lb *LoadBalancer) AgentFinalizerName(nodeName string) string {
	return nodeName + "." + loadbalancerAgentFinalizerName
}

// AddDNSEndpoint adds an external-dns Endpoint struct to the LB's
// Spec.Endpoints. The Endpoint is based on the LBSG's template and
// the DNS name is generated from a template. Two parameters are
// provided to the template: .LBName and .LBSGName.
func (lb *LoadBalancer) AddDNSEndpoint(lbsg LBServiceGroup) error {
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
		LBName            string
		LBSGName          string
		PureLBServiceName string
		IPAddress         string
	}{
		lb.Name,
		lbsg.Name,
		lb.Spec.DisplayName,
		rfc1123Cleaner.Replace(lb.Spec.PublicAddress),
	}
	err = tmpl.Execute(&doc, params)
	if err != nil {
		return err
	}

	// Fill in the Endpoint
	ep := Endpoint{}
	lbsg.Spec.EndpointTemplate.DeepCopyInto(&ep)
	ep.DNSName = doc.String()
	ep.Targets = append(ep.Targets, lb.Spec.PublicAddress)

	// Add the Endpoint to the list
	lb.Spec.Endpoints = append(lb.Spec.Endpoints, &ep)

	return nil
}

// getLBServiceGroup gets this LB's owning LBServiceGroup.
func (lb *LoadBalancer) getLBServiceGroup(ctx context.Context, cl client.Client) (*LBServiceGroup, error) {
	if lb.Labels[OwningLBServiceGroupLabel] == "" {
		return nil, fmt.Errorf("LB has no owning service group label")
	}
	sgName := types.NamespacedName{Namespace: lb.Namespace, Name: lb.Labels[OwningLBServiceGroupLabel]}
	sg := &LBServiceGroup{}
	return sg, cl.Get(ctx, sgName, sg)
}

func init() {
	SchemeBuilder.Register(&LoadBalancer{}, &LoadBalancerList{})
}
