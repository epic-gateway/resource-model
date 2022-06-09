package v1

import (
	"crypto/rand"
	"encoding/base64"
	"net"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	lbLog     = logf.Log.WithName("loadbalancer-resource")
	allocator PoolAllocator
)

// PoolAllocator allocates addresses. We use an interface to avoid
// import loops between the v1 package and the allocator package.
//
// +kubebuilder:object:generate=false
type PoolAllocator interface {
	AllocateFromPool(string, string, []corev1.ServicePort, string) (net.IP, error)
}

// generateTunnelKey generates a 128-bit tunnel key and returns it as
// a base64-encoded string.
func generateTunnelKey() string {
	raw := make([]byte, 16, 16)
	_, _ = rand.Read(raw)
	return base64.StdEncoding.EncodeToString(raw)
}
