package v1

// ClientRef provides the info needed to refer to a specific object in
// a specific cluster.
type ClientRef struct {
	ClusterID string `json:"clusterID,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	UID       string `json:"uid,omitempty"`
}
