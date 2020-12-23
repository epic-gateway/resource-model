package v1

const (
	// MetricsNamespace is the namespace used with Prometheus.
	MetricsNamespace string = "egw"

	// AccountNamespace is the namespace where we store Account objects.
	AccountNamespace string = "egw"

	// ConfigNamespace is the namespace where we store the system
	// configuration objects like the EGW singleton and the node
	// configs.
	ConfigNamespace string = "egw"

	// AccountNamespacePrefix is the string prepended to all account
	// names to form the namespace in which that account's resources are
	// stored. If an account were called "caboteria" then its namespace
	// would be "egw-caboteria".
	AccountNamespacePrefix string = "egw-"
)
