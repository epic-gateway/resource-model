package v1

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
)
