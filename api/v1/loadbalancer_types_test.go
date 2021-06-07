package v1

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContains(t *testing.T) {
	lb := LoadBalancer{
		Spec: LoadBalancerSpec{
			UpstreamClusters: []string{"foo", "bar"},
		},
	}

	assert.True(t, lb.ContainsUpstream("foo"), "ContainsUpstream false negative")
	assert.False(t, lb.ContainsUpstream("nope"), "ContainsUpstream false positive")
}

func TestAdd(t *testing.T) {
	lb := LoadBalancer{
		Spec: LoadBalancerSpec{
			UpstreamClusters: []string{"foo", "bar"},
		},
	}

	assert.Nil(t, lb.AddUpstream("fred"), "Adding a new upstream should return nil")
	assert.ElementsMatch(t, []string{"foo", "bar", "fred"}, lb.Spec.UpstreamClusters, "unexpected set of upstreams")
	assert.Error(t, lb.AddUpstream("fred"), "Adding a duplicate upstream should return an error")
	assert.ElementsMatch(t, []string{"foo", "bar", "fred"}, lb.Spec.UpstreamClusters, "unexpected set of upstreams")
}

func TestRemove(t *testing.T) {
	lb := LoadBalancer{
		Spec: LoadBalancerSpec{
			UpstreamClusters: []string{"foo", "bar", "baz"},
		},
	}

	assert.Nil(t, lb.RemoveUpstream("bar"), "Removing a known upstream should return nil")
	assert.ElementsMatch(t, []string{"foo", "baz"}, lb.Spec.UpstreamClusters, "unexpected set of upstreams")
	assert.Error(t, lb.RemoveUpstream("bar"), "Removing an unknown upstream should return an error")
	assert.ElementsMatch(t, []string{"foo", "baz"}, lb.Spec.UpstreamClusters, "unexpected set of upstreams")
}

func TestLoadBalancerName(t *testing.T) {
	// "Shareable" LBs have simple names based only on the SG and raw name
	assert.EqualValues(t, "group-lb", LoadBalancerName("group", "lb", true), "wrong name for shareable LB")

	// Non-shareable LBs have a random hex suffix at the end of their names
	assert.Regexp(t, regexp.MustCompile("group-lb-.[0-9|a-f]"), LoadBalancerName("group", "lb", false), "wrong name for non-shareable LB")
}
