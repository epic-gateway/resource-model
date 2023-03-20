package envoy

import (
	"encoding/json"
	"fmt"
	"testing"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/contour/gatewayapi"
)

var (
	prefix    gatewayv1a2.PathMatchType       = gatewayv1a2.PathMatchPathPrefix
	exact     gatewayv1a2.PathMatchType       = gatewayv1a2.PathMatchExact
	get       gatewayv1a2.HTTPMethod          = gatewayv1a2.HTTPMethodGet
	regex     gatewayv1a2.QueryParamMatchType = gatewayv1a2.QueryParamMatchRegularExpression
	proxyName gatewayv1a2.Hostname            = "*.unit-test"
)

const (
	clusterConfigSample = `name: purelb
connect_timeout: 2s
type: STRICT_DNS
lb_policy: ROUND_ROBIN
load_assignment:
  cluster_name: purelb
{{- if .Endpoints}}
  endpoints:
  - lb_endpoints:
{{- range .Endpoints}}
    - endpoint:
        address:
          socket_address:
            address: {{.Spec.Address}}
            protocol: {{.Spec.Port.Protocol | ToUpper}}
            port_value: {{.Spec.Port.Port}}
{{- end}}
{{- end}}
`
	listenerConfigSample = `name: {{.PortName}}
address:
  socket_address:
    address: 0.0.0.0
    port_value: {{.Port}}
    protocol: {{.Protocol | ToUpper}}
filter_chains:
  - filters:
    - name: envoy.http_connection_manager
      typed_config:
        "@type": type.googleapis.com/envoy.config.filter.network.http_connection_manager.v2.HttpConnectionManager
        stat_prefix: {{ .ServiceName }}
        route_config:
          name: local_route
          virtual_hosts:
            - name: purelb
              domains: ["*"]
              routes:
                - match:
                    prefix: "/"
                  route:
                    cluster: purelb
        http_filters:
          - name: envoy.filters.http.router
`
)

var (
	testService = epicv1.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: epicv1.LoadBalancerSpec{
			EnvoyTemplate: &marin3r.EnvoyConfigSpec{
				EnvoyResources: &marin3r.EnvoyResources{
					Clusters: []marin3r.EnvoyResource{{
						Value: clusterConfigSample,
					}},
					Listeners: []marin3r.EnvoyResource{{
						Value: listenerConfigSample,
					}},
				},
			},
			UpstreamClusters: []string{"fred", "barney", "betty"},
		},
	}

	catchall = gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Path: &gatewayv1a2.HTTPPathMatch{
				Type:  &prefix,
				Value: pointer.StringPtr("/"),
			},
		}},
		BackendRefs: []gatewayv1a2.HTTPBackendRef{},
	}

	one_match = gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Path: &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/api")},
		}},
		BackendRefs: []gatewayv1a2.HTTPBackendRef{
			{
				BackendRef: gatewayv1a2.BackendRef{
					BackendObjectReference: gatewayv1a2.BackendObjectReference{
						Name: "one_match",
					},
					Weight: pointer.Int32(1),
				},
				Filters: []gatewayv1a2.HTTPRouteFilter{},
			},
		},
	}

	two_matches = gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Method: &get,
			Path:   &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/web")},
		}},
		BackendRefs: []gatewayv1a2.HTTPBackendRef{
			{
				BackendRef: gatewayv1a2.BackendRef{
					BackendObjectReference: gatewayv1a2.BackendObjectReference{Name: "two_matches"},
					Weight:                 pointer.Int32(1),
				},
			},
		},
	}
)

func TestServiceToCluster(t *testing.T) {
	cluster, err := ServiceToCluster(testService, []epicv1.RemoteEndpoint{})
	assert.Nil(t, err, "template processing failed")
	fmt.Println(cluster)

	cluster, err = ServiceToCluster(testService, []epicv1.RemoteEndpoint{{
		Spec: epicv1.RemoteEndpointSpec{
			Address: "1.1.1.1",
			Port: corev1.EndpointPort{
				Port:     42,
				Protocol: "udp",
			},
		},
	}})
	if err != nil {
		fmt.Printf("********************** %#v\n\n", err.Error())
	}
	assert.Nil(t, err, "template processing failed")
	fmt.Println(cluster)
}

func TestMakeHTTPListener(t *testing.T) {
	listener, err := makeHTTPListener(listenerConfigSample, testService, corev1.ServicePort{
		Protocol: "tcp",
		Port:     42,
	})
	assert.Nil(t, err, "template processing failed")
	fmt.Println(listener)
}

func TestSortRouteRules(t *testing.T) {
	// This Route has a more specific match after a less specific one so
	// they should be reversed
	raw := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: &gatewayv1a2.HTTPRouteSpec{
				Rules: []gatewayv1a2.HTTPRouteRule{catchall, two_matches, one_match},
			},
		},
	}
	want := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: &gatewayv1a2.HTTPRouteSpec{
				Rules: []gatewayv1a2.HTTPRouteRule{two_matches, one_match, catchall},
			},
		},
	}
	cooked, err := sortRouteRules(raw)
	assert.Nil(t, err, "sortRouteRules failed")

	assert.Equal(t, jsonify(t, want), jsonify(t, cooked))
}

func TestPreprocessHTTPRoutes(t *testing.T) {
	rule2 := gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Path: &gatewayv1a2.HTTPPathMatch{
				Type:  &exact,
				Value: pointer.StringPtr("/rule2"),
			},
		}},
		BackendRefs: []gatewayv1a2.HTTPBackendRef{},
	}

	// Trivial case: empty route slice
	raw := []epicv1.GWRoute{}
	want := []epicv1.GWRoute{}
	cooked, err := preprocessHTTPRoutes(nil, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.Equal(t, want, cooked)

	// Test Route re-ordering. The first input route has a catchall rule
	// so it should be last in the output slice.
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname, and neither does the
					// listener, so this should get "*" added to it
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"*"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = preprocessHTTPRoutes(nil, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()

	// Test Routes "borrowing" Proxy hostnames, i.e., Routes that have
	// no Hostname referencing a Listener that does have a Hostname.
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname but the listener does so this
					// should "borrow" the listener's hostname
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test", "host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{proxyName},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = preprocessHTTPRoutes(&proxyName, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()

	// Test pruning Route hostnames to only ones that match the
	// listener.
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname but the listener does so this
					// should "borrow" the listener's hostname
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					// This route won't appear at all in the output since its
					// Hostname doesn't match.
					Hostnames: []gatewayv1a2.Hostname{"example.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"foo.bar", "host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{proxyName},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = preprocessHTTPRoutes(&proxyName, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()

	// Test combining Route Rules. If two Rules within a Route have the
	// same Filters and Hostnames then they can be combined, i.e.,
	// we'll create one output Route with both of the input Routes'
	// BackendRefs.
	two_matches_alt := two_matches.DeepCopy()
	two_matches_alt.BackendRefs[0].BackendObjectReference.Name = "alt_match"
	raw = []epicv1.GWRoute{

		// These two GWRoutes should combine to a single GWRoute with one
		// HTTPRouteRule that has two HTTPBackendRefs.
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{two_matches},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{*two_matches_alt},
				},
			},
		},

		// This GWRoute should pass through to the output.
		{
			Spec: epicv1.GWRouteSpec{ // This should combine with the first one
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"foo.bar", "host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, two_matches},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
					Rules: []gatewayv1a2.HTTPRouteRule{
						{
							Matches: []gatewayv1a2.HTTPRouteMatch{{
								Method: &get,
								Path:   &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/web")},
							}},
							BackendRefs: []gatewayv1a2.HTTPBackendRef{
								{
									BackendRef: gatewayv1a2.BackendRef{
										BackendObjectReference: gatewayv1a2.BackendObjectReference{Name: "two_matches"},
										Weight:                 pointer.Int32(1),
									},
								},
								{
									BackendRef: gatewayv1a2.BackendRef{
										BackendObjectReference: gatewayv1a2.BackendObjectReference{Name: "alt_match"},
										Weight:                 pointer.Int32(1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{two_matches, rule2}, // NOTE: catchall rule is now last
				},
			},
		},
	}
	cooked, err = preprocessHTTPRoutes(&proxyName, raw)
	assert.NoError(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked)
}

func TestPreprocessRoutes(t *testing.T) {
	listener := gatewayv1a2.Listener{Name: "unit-test"}

	// Trivial case: empty route slice
	raw := []epicv1.GWRoute{}
	want := []epicv1.GWRoute{}
	cooked, err := preprocessRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.Equal(t, want, cooked)

	// TCPRoutes are passed through
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				TCP: &gatewayv1a2.TCPRouteSpec{
					// This route has no hostname, and neither does the
					// listener, so this should get "*" added to it
					Rules: []gatewayv1a2.TCPRouteRule{},
				},
			},
		},
	}
	want = []epicv1.GWRoute{*raw[0].DeepCopy()}
	cooked, err = preprocessRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.Equal(t, want, cooked)

	// HTTPRoutes are pre-processed
	listener = gatewayv1a2.Listener{
		Name:     "prune-test",
		Hostname: &proxyName,
	}
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname but the listener does so this
					// should "borrow" the listener's hostname
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				TCP: &gatewayv1a2.TCPRouteSpec{
					// This route has no hostname, and neither does the
					// listener, so this should get "*" added to it
					Rules: []gatewayv1a2.TCPRouteRule{},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				TCP: &gatewayv1a2.TCPRouteSpec{
					// This route has no hostname, and neither does the
					// listener, so this should get "*" added to it
					Rules: []gatewayv1a2.TCPRouteRule{},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{proxyName},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = preprocessRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()
}

func TestCombineRouteRules(t *testing.T) {
	// If two Rules within a Route have the same Filters and Hostnames
	// then they can be combined, i.e., we'll create one output Route
	// with both of the input Routes' BackendRefs.
	two_matches_alt := two_matches.DeepCopy()
	two_matches_alt.BackendRefs[0].BackendObjectReference.Name = "alt_match"
	two_matches_bis := two_matches.DeepCopy()
	two_matches_bis.BackendRefs[0].BackendObjectReference.Name = "bis_match"
	raw := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: &gatewayv1a2.HTTPRouteSpec{
				Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
				Rules:     []gatewayv1a2.HTTPRouteRule{two_matches, *two_matches_alt, *two_matches_bis},
			},
		},
	}
	want := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: &gatewayv1a2.HTTPRouteSpec{
				Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
				Rules: []gatewayv1a2.HTTPRouteRule{
					{
						Matches: []gatewayv1a2.HTTPRouteMatch{{
							Method: &get,
							Path:   &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/web")},
						}},
						BackendRefs: []gatewayv1a2.HTTPBackendRef{
							{
								BackendRef: gatewayv1a2.BackendRef{
									BackendObjectReference: gatewayv1a2.BackendObjectReference{Name: "two_matches"},
									Weight:                 pointer.Int32(1),
								},
							},
							{
								BackendRef: gatewayv1a2.BackendRef{
									BackendObjectReference: gatewayv1a2.BackendObjectReference{Name: "alt_match"},
									Weight:                 pointer.Int32(1),
								},
							},
							{
								BackendRef: gatewayv1a2.BackendRef{
									BackendObjectReference: gatewayv1a2.BackendObjectReference{Name: "bis_match"},
									Weight:                 pointer.Int32(1),
								},
							},
						},
					},
				},
			},
		},
	}
	cooked, err := combineRouteRules(raw)
	assert.NoError(t, err, "route preprocessing failed")
	assert.Equal(t, jsonify(t, want), jsonify(t, cooked))
}

func TestShouldMerge(t *testing.T) {
	// one_match and two_matches have different HTTPRouteMatch so they
	// can't merge.
	assert.False(t, shouldMerge(one_match, two_matches))

	// The same object should merge (although this would probably never
	// happen IRL).
	one_match_alt := one_match.DeepCopy()
	assert.True(t, shouldMerge(one_match, *one_match_alt))
}

func TestIsCatchallMatch(t *testing.T) {
	assert.False(t, isCatchall(gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Path: &gatewayv1a2.HTTPPathMatch{
				Type:  &prefix,
				Value: pointer.StringPtr("/not-catchall"),
			},
		}},
	}))

	assert.True(t, isCatchall(gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Path: &gatewayv1a2.HTTPPathMatch{
				Type:  &prefix,
				Value: pointer.StringPtr("/"),
			},
		}},
	}))
}

func TestCriteriaCount(t *testing.T) {
	assert.Equal(t, 1, criteriaCount(gatewayv1a2.HTTPRouteMatch{
		Path: &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/prefix")},
	}))
	assert.Equal(t, 1, criteriaCount(gatewayv1a2.HTTPRouteMatch{
		Headers: []gatewayv1a2.HTTPHeaderMatch{{Name: "test", Value: "unit"}},
	}))
	assert.Equal(t, 2, criteriaCount(gatewayv1a2.HTTPRouteMatch{
		Path:    &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/prefix")},
		Headers: []gatewayv1a2.HTTPHeaderMatch{{Name: "test", Value: "unit"}},
	}))
	assert.Equal(t, 4, criteriaCount(gatewayv1a2.HTTPRouteMatch{
		QueryParams: []gatewayv1a2.HTTPQueryParamMatch{{Type: &regex, Name: "test", Value: "^unit$"}},
		Method:      &get,
		Path:        &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/prefix")},
		Headers:     []gatewayv1a2.HTTPHeaderMatch{{Name: "test", Value: "unit"}},
	}))
}

func TestSplitMatches(t *testing.T) {
	got := []gatewayv1a2.HTTPRouteRule{{
		Matches: []gatewayv1a2.HTTPRouteMatch{
			{Method: &get},
			{Path: &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/web")}},
		},
	}}
	want := []gatewayv1a2.HTTPRouteRule{{
		Matches: []gatewayv1a2.HTTPRouteMatch{
			{Method: &get},
		},
	}, {
		Matches: []gatewayv1a2.HTTPRouteMatch{
			{Path: &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/web")}},
		},
	}}

	split := splitMatches(got)
	assert.Equal(t, 2, len(split))

	assert.Equal(t, jsonify(t, want), jsonify(t, split))
}

func TestCompatibleListeners(t *testing.T) {
	assert.False(t, compatibleListeners(
		gatewayv1a2.Listener{Port: 42},
		gatewayv1a2.Listener{Port: 27}), "port mismatch should be incompatible")
	assert.True(t, compatibleListeners(
		gatewayv1a2.Listener{Protocol: gatewayv1a2.HTTPSProtocolType},
		gatewayv1a2.Listener{Protocol: gatewayv1a2.HTTPSProtocolType}), "same protocol should be compatible")
	assert.True(t, compatibleListeners(
		gatewayv1a2.Listener{Protocol: gatewayv1a2.TLSProtocolType},
		gatewayv1a2.Listener{Protocol: gatewayv1a2.HTTPSProtocolType}), "HTTPS/TLS protocols should be compatible")
	assert.False(t, compatibleListeners(
		gatewayv1a2.Listener{Protocol: gatewayv1a2.TCPProtocolType},
		gatewayv1a2.Listener{Protocol: gatewayv1a2.HTTPSProtocolType}), "protocol mismatch should be incompatible")
}

func TestFilterReferentRoutes(t *testing.T) {
	listener := gatewayv1a2.Listener{Name: "unit-test"}
	matches := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: &gatewayv1a2.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1a2.CommonRouteSpec{ParentRefs: []gatewayv1a2.ParentReference{{}}},
			},
		},
	}

	// Trivial case: empty route slice
	raw := []epicv1.GWRoute{}
	want := []epicv1.GWRoute{}
	cooked, err := filterReferentRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.Equal(t, want, cooked)

	// Two input routes: one matches, one doesn't. Output should be only
	// the matching route.
	raw = []epicv1.GWRoute{
		matches,
		{
			// This should not be in the output since its sectionName
			// doesn't match.
			Spec: epicv1.GWRouteSpec{
				HTTP: &gatewayv1a2.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1a2.CommonRouteSpec{
						ParentRefs: []gatewayv1a2.ParentReference{{SectionName: gatewayapi.SectionNamePtr("not-unit-test")}},
					},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		matches,
	}
	cooked, err = filterReferentRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()
}

func jsonify(t *testing.T, obj interface{}) string {
	bytes, err := json.MarshalIndent(obj, "", " ")
	if err != nil {
		assert.Fail(t, "Failure marshaling to json", obj)
	}
	return string(bytes)
}
