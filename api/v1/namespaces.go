package v1

const (
	// MetricsNamespace is the namespace used with Prometheus.
	MetricsNamespace string = ProductName

	// ConfigNamespace is the namespace where we store the system
	// configuration objects like the EPIC singleton and the service
	// prefixes.
	ConfigNamespace string = ProductName

	// UserNamespacePrefix is the prefix used by all namespaces that are
	// EPIC User Namespaces.
	UserNamespacePrefix string = ProductName + "-"
)
