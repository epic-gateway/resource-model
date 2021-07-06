package v1

import v1 "k8s.io/api/core/v1"

const (
	// OwningAccountLabel is the name of the label that we apply to
	// service groups and load balancers to indicate in a query-friendly
	// way to which Account they belong.
	OwningAccountLabel string = GroupName + "/owning-account"

	// OwningLBServiceGroupLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// LBServiceGroup they belong.
	OwningLBServiceGroupLabel string = GroupName + "/owning-lbservicegroup"

	// OwningServicePrefixLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// ServicePrefix they belong.
	OwningServicePrefixLabel string = GroupName + "/owning-serviceprefix"

	// OwningLoadBalancerLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// LoadBalancer they belong.
	OwningLoadBalancerLabel string = GroupName + "/owning-loadbalancer"

	// OwningClusterLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which Cluster
	// they belong.
	OwningClusterLabel string = GroupName + "/owning-cluster"
)

var (
	// UserNSLabels is the set of labels that indicate that a k8s
	// namespace is an EPIC User Namespace.
	UserNSLabels = map[string]string{"app.kubernetes.io/component": "user-namespace", "app.kubernetes.io/part-of": ProductName}

	// envoyProxyLabels is the set of labels that we apply to one of our
	// LoadBalancer CRs. Note that if these change then you'll need to
	// update the python setup-network program so it matches.
	envoyProxyLabels = map[string]string{
		"app.kubernetes.io/name":      "envoy",
		"app.kubernetes.io/part-of":   ProductName,
		"app.kubernetes.io/component": "proxy",
		OwningLoadBalancerLabel:       "PLACEHOLDER"}
)

// LabelsForEnvoy returns the labels that we apply to a new Envoy
// proxy deployment.
func LabelsForEnvoy(name string) map[string]string {
	// Copy the template label map
	labels := make(map[string]string, len(envoyProxyLabels))
	for k, v := range envoyProxyLabels {
		labels[k] = v
	}

	// Override the owning LB placeholder with our actual owning LB
	labels[OwningLoadBalancerLabel] = name

	return labels
}

// HasEnvoyLabels indicates whether a Pod has the LabelsForEnvoy,
// i.e., whether the Pod is an Envoy proxy pod.
func HasEnvoyLabels(pod v1.Pod) bool {
	partOf, hasPartOf := pod.ObjectMeta.Labels["app.kubernetes.io/part-of"]
	if !hasPartOf {
		return false
	}
	if partOf != ProductName {
		return false
	}

	name, hasName := pod.ObjectMeta.Labels["app.kubernetes.io/name"]
	if !hasName {
		return false
	}
	if name != "envoy" {
		return false
	}

	component, hasComponent := pod.ObjectMeta.Labels["app.kubernetes.io/component"]
	if !hasComponent {
		return false
	}
	if component != "proxy" {
		return false
	}

	_, hasOwner := pod.ObjectMeta.Labels[OwningLoadBalancerLabel]
	if !hasOwner {
		return false
	}

	return true
}
