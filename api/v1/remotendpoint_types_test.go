package v1

import (
	"net"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestRemoteEndpointName(t *testing.T) {
	// Reps have a random hex suffix at the end of their names
	assert.Regexp(t, regexp.MustCompile("fc00-f853-ccd-e793--1-4242-.[0-9|a-f]"), RemoteEndpointName(net.ParseIP("fc00:f853:ccd:e793::1"), 4242, v1.ProtocolTCP), "wrong name for non-shareable LB")
}
