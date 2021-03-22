package v1

const (
	// OwningAccountLabel is the name of the label that we apply to
	// service groups and load balancers to indicate in a query-friendly
	// way to which Account they belong.
	OwningAccountLabel string = "owning-account"

	// OwningServiceGroupLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// ServiceGroup they belong.
	OwningServiceGroupLabel string = "owning-servicegroup"

	// OwningServicePrefixLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// ServicePrefix they belong.
	OwningServicePrefixLabel string = "owning-serviceprefix"

	// OwningLoadBalancerLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which
	// LoadBalancer they belong.
	OwningLoadBalancerLabel string = "owning-loadbalancer"

	// OwningClusterLabel is the name of the label that we apply to
	// endpoints to indicate in a query-friendly way to which Cluster
	// they belong.
	OwningClusterLabel string = "owning-cluster"
)
