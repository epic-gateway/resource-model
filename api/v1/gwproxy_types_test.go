package v1

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGWProxyName(t *testing.T) {
	// "Shareable" LBs have simple names based only on the SG and raw name
	assert.EqualValues(t, "group-lb", GWProxyName("group", "lb", true), "wrong name for shareable LB")

	// Non-shareable LBs have a random hex suffix at the end of their names
	assert.Regexp(t, regexp.MustCompile("group-lb-.[0-9|a-f]"), GWProxyName("group", "lb", false), "wrong name for non-shareable LB")
}

func TestGWAddDNSEndpoint(t *testing.T) {
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

	lb := GWProxy{
		Spec: GWProxySpec{},
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
			template: "{{.IPAddress}}",
			expected: "10-42-27-9-70b4",
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
			template: "{{.IPAddress}}.{{.ClusterServiceName}}.{{.LBSGName}}.example.com",
			expected: "10-42-27-9-70b4.TestService.TestSG.example.com",
		},
	} {
		lb := GWProxy{
			ObjectMeta: metav1.ObjectMeta{
				Name: "TestLB",
			},
			Spec: GWProxySpec{
				DisplayName: "TestService",
				// This is the demon-spawn of an unholy union between IPV4 and
				// IPV6. It's not a real address but should contain all of the
				// special characters that our RFC1123 cleaner is supposed to
				// replace
				PublicAddress: "10.42.27.9:70b4",
			},
		}

		sg.Spec.EndpointTemplate.DNSName = tc.template
		assert.NoError(t, lb.AddDNSEndpoint(sg), "Failed to add DNS Endpoint")
		assert.Equal(t, tc.expected, lb.Spec.Endpoints[0].DNSName, "Incorrect template result")
	}
}
