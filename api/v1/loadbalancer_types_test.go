package v1

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestAddDNSEndpoint(t *testing.T) {
	sg := LBServiceGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "TestSG",
		},
		Spec: LBServiceGroupSpec{
			EndpointTemplate: Endpoint{
				DNSName:    "",
				RecordType: "TestType",
				RecordTTL:  42,
				Labels:     map[string]string{"key": "value"},
				ProviderSpecific: []ProviderSpecificProperty{{
					Name:  "TestName",
					Value: "TestValue",
				}},
			},
		},
	}

	lb := LoadBalancer{
		Spec: LoadBalancerSpec{},
	}

	assert.NoError(t, lb.AddDNSEndpoint(sg), "Failed to add DNS Endpoint")
	assert.Equal(t, TTL(42), lb.Spec.Endpoints[0].RecordTTL, "Incorrect RecordTTL")
	assert.Equal(t, "TestType", lb.Spec.Endpoints[0].RecordType, "Incorrect RecordType")
	assert.EqualValues(t, sg.Spec.EndpointTemplate.Labels, lb.Spec.Endpoints[0].Labels, "Incorrect labels")
	assert.EqualValues(t, sg.Spec.EndpointTemplate.ProviderSpecific, lb.Spec.Endpoints[0].ProviderSpecific, "Incorrect ProviderSpecific")

	// Test template formatting
	for _, tc := range []struct {
		template string
		expected string
	}{
		{
			template: "",
			expected: "",
		},
		{
			template: "test",
			expected: "test",
		},
		{
			template: "{{.LBName}}",
			expected: "TestLB",
		},
		{
			template: "{{.LBSGName}}",
			expected: "TestSG",
		},
		{
			template: "{{.PureLBServiceName}}.{{.LBSGName}}.example.com",
			expected: "TestService.TestSG.example.com",
		},
	} {
		lb := LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{
				Name: "TestLB",
			},
			Spec: LoadBalancerSpec{
				DisplayName: "TestService",
			},
		}

		sg.Spec.EndpointTemplate.DNSName = tc.template
		assert.NoError(t, lb.AddDNSEndpoint(sg), "Failed to add DNS Endpoint")
		assert.Equal(t, tc.expected, lb.Spec.Endpoints[0].DNSName, "Incorrect template result")
	}
}
