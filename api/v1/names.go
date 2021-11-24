package v1

const (
	// ProductName is the name of our product. It's EPIC!
	ProductName = "epic"

	// ConfigName is the name of the EPIC configuration singleton. Its
	// namespace is defined in namespaces.go.
	ConfigName = ProductName

	// EDSServerName is the name of our dynamic endpoint discovery
	// service.
	EDSServerName string = "eds-server"

	// DiscoveryServiceName is the name of the Marin3r DiscoveryService
	// CR that we create in each customer namespace to tell Marin3r to
	// launch its discoveryservice in that namespace.
	DiscoveryServiceName string = "discoveryservice"
)
