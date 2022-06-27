package envoy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1beta1"

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
		BackendRefs: []gatewayv1a2.HTTPBackendRef{},
	}

	two_matches = gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Method: &get,
			Path:   &gatewayv1a2.HTTPPathMatch{Type: &prefix, Value: pointer.StringPtr("/web")},
		}},
		BackendRefs: []gatewayv1a2.HTTPBackendRef{},
	}
)

func TestSortRouteRules(t *testing.T) {
	// This Route has a more specific match after a less specific one so
	// they should be reversed
	raw := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: gatewayv1a2.HTTPRouteSpec{
				Rules: []gatewayv1a2.HTTPRouteRule{catchall, two_matches, one_match},
			},
		},
	}
	want := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: gatewayv1a2.HTTPRouteSpec{
				Rules: []gatewayv1a2.HTTPRouteRule{two_matches, one_match, catchall},
			},
		},
	}
	cooked, err := sortRouteRules(raw)
	assert.Nil(t, err, "sortRouteRules failed")

	assert.Equal(t, jsonify(t, want), jsonify(t, cooked))
}

func TestPreprocessRoutes(t *testing.T) {
	listener := gatewayv1a2.Listener{Name: "unit-test"}
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
	cooked, err := preprocessRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.Equal(t, want, cooked)

	// Test Route re-ordering. The first input route has a catchall rule
	// so it should be last in the output slice.
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname, and neither does the
					// listener, so this should get "*" added to it
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"*"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = preprocessRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()

	// Test Routes "borrowing" Proxy hostnames, i.e., Routes that have
	// no Hostname referencing a Listener that does have a Hostname.
	listener = gatewayv1a2.Listener{
		Hostname: &proxyName,
	}
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname but the listener does so this
					// should "borrow" the listener's hostname
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test", "host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{proxyName},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = preprocessRoutes(listener, raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.ElementsMatch(t, want, cooked) // FIXME: order matters here, need to use assert.Equal()

	// Test pruning Route hostnames to only ones that match the
	// listener.
	listener = gatewayv1a2.Listener{
		Hostname: &proxyName,
	}
	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					// This route has no hostname but the listener does so this
					// should "borrow" the listener's hostname
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					// This route won't appear at all in the output since its
					// Hostname doesn't match.
					Hostnames: []gatewayv1a2.Hostname{"example.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"acnodal.io", "host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"foo.bar", "host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host1.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
					Hostnames: []gatewayv1a2.Hostname{"host2.unit-test"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: gatewayv1a2.HTTPRouteSpec{
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
			HTTP: gatewayv1a2.HTTPRouteSpec{
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
				HTTP: gatewayv1a2.HTTPRouteSpec{
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
	bytes, err := json.Marshal(obj)
	if err != nil {
		assert.Fail(t, "Failure marshaling to json", obj)
	}
	return string(bytes)
}
