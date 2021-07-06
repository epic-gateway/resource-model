package v1

const (
	// IfIndexAnnotation is the key for the Pod annotation that contains
	// that Pod's proxy network interface index.
	IfIndexAnnotation string = "epic.acnodal.io/ifindex"

	// IfNameAnnotation is the key for the Pod annotation that contains
	// that Pod's proxy network interface name.
	IfNameAnnotation string = "epic.acnodal.io/ifname"
)
