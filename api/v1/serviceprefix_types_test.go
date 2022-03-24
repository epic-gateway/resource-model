package v1

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAggregateRoute(t *testing.T) {
	var (
		err      error
		network  net.IPNet
		target   *net.IPNet
		ip4, ip6 net.IP
		sps      AddressPool
	)

	// A few error cases. Should return err, not crash
	ip4, target, err = net.ParseCIDR("10.42.27.6/16")
	sps = AddressPool{
		Subnet:      "blah",
		Aggregation: "default",
	}
	_, err = sps.aggregateRoute(ip4)
	assert.NotNil(t, err, "Should have received an error")
	sps = AddressPool{
		Subnet:      "10.42.0.0/16",
		Aggregation: "blorp",
	}
	_, err = sps.aggregateRoute(ip4)
	assert.NotNil(t, err, "Should have received an error")

	// Default aggregation
	sps = AddressPool{
		Subnet:      "10.42.0.0/16",
		Aggregation: "default",
	}
	ip4, target, err = net.ParseCIDR("10.42.27.6/16")
	network, err = sps.aggregateRoute(ip4)
	assert.Nil(t, err, "error aggregating")
	assert.Equal(t, *target, network)

	sps = AddressPool{
		Subnet:      "fe80::fb2d:1eb8:0:0/96",
		Aggregation: "default",
	}
	ip6, target, err = net.ParseCIDR("fe80::fb2d:1eb8:addb:6a66/96")
	network, err = sps.aggregateRoute(ip6)
	assert.Nil(t, err, "error aggregating")
	assert.Equal(t, *target, network)

	// Explicit aggregation, more specific than the subnet
	sps = AddressPool{
		Subnet:      "10.42.0.0/16",
		Aggregation: "/32",
	}
	ip4, target, err = net.ParseCIDR("10.42.27.6/32")
	network, err = sps.aggregateRoute(ip4)
	assert.Nil(t, err, "error aggregating")
	assert.Equal(t, *target, network)

	sps = AddressPool{
		Subnet:      "fe80::fb2d:1eb8:0:0/96",
		Aggregation: "/128",
	}
	ip6, target, err = net.ParseCIDR("fe80::fb2d:1eb8:addb:6a66/128")
	network, err = sps.aggregateRoute(ip6)
	assert.Nil(t, err, "error aggregating")
	assert.Equal(t, *target, network)

	// Explicit aggregation, less specific than the subnet
	sps = AddressPool{
		Subnet:      "10.42.0.0/16",
		Aggregation: "/8",
	}
	ip4, target, err = net.ParseCIDR("10.42.27.6/8")
	network, err = sps.aggregateRoute(ip4)
	assert.Nil(t, err, "error aggregating")
	assert.Equal(t, *target, network)

	sps = AddressPool{
		Subnet:      "fe80::fb2d:1eb8:0:0/96",
		Aggregation: "/64",
	}
	ip6, target, err = net.ParseCIDR("fe80::fb2d:1eb8:addb:6a66/64")
	network, err = sps.aggregateRoute(ip6)
	assert.Nil(t, err, "error aggregating")
	assert.Equal(t, *target, network)
}
