package envoy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	prefix gatewayv1a2.PathMatchType       = gatewayv1a2.PathMatchPathPrefix
	exact  gatewayv1a2.PathMatchType       = gatewayv1a2.PathMatchExact
	get    gatewayv1a2.HTTPMethod          = gatewayv1a2.HTTPMethodGet
	regex  gatewayv1a2.QueryParamMatchType = gatewayv1a2.QueryParamMatchRegularExpression
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
			HTTP: v1alpha2.HTTPRouteSpec{
				Rules: []gatewayv1a2.HTTPRouteRule{catchall, two_matches, one_match},
			},
		},
	}
	want := epicv1.GWRoute{
		Spec: epicv1.GWRouteSpec{
			HTTP: v1alpha2.HTTPRouteSpec{
				Rules: []gatewayv1a2.HTTPRouteRule{two_matches, one_match, catchall},
			},
		},
	}
	cooked, err := sortRouteRules(raw)
	assert.Nil(t, err, "sortRouteRules failed")

	assert.Equal(t, jsonify(t, want), jsonify(t, cooked))
}

func TestPreprocessRoutes(t *testing.T) {
	// Trivial case: empty route slice
	raw := []epicv1.GWRoute{}
	want := []epicv1.GWRoute{}
	cooked, err := PreprocessRoutes(raw)
	assert.Nil(t, err, "route preprocessing failed")
	assert.Equal(t, want, cooked)
	rule2 := gatewayv1a2.HTTPRouteRule{
		Matches: []gatewayv1a2.HTTPRouteMatch{{
			Path: &gatewayv1a2.HTTPPathMatch{
				Type:  &exact,
				Value: pointer.StringPtr("/rule2"),
			},
		}},
		BackendRefs: []gatewayv1a2.HTTPBackendRef{},
	}

	raw = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: v1alpha2.HTTPRouteSpec{
					Rules: []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: v1alpha2.HTTPRouteSpec{
					Hostnames: []v1alpha2.Hostname{"acnodal.io", "acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: v1alpha2.HTTPRouteSpec{
					Hostnames: []v1alpha2.Hostname{"acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2},
				},
			},
		},
	}
	want = []epicv1.GWRoute{
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: v1alpha2.HTTPRouteSpec{
					Hostnames: []v1alpha2.Hostname{"acnodal.com"},
					Rules:     []gatewayv1a2.HTTPRouteRule{rule2, catchall}, // NOTE: catchall rule is now last
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: v1alpha2.HTTPRouteSpec{
					Hostnames: []v1alpha2.Hostname{"acnodal.io"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
		{
			Spec: epicv1.GWRouteSpec{
				HTTP: v1alpha2.HTTPRouteSpec{
					Hostnames: []v1alpha2.Hostname{"*"},
					Rules:     []gatewayv1a2.HTTPRouteRule{catchall},
				},
			},
		},
	}
	cooked, err = PreprocessRoutes(raw)
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

func jsonify(t *testing.T, obj interface{}) string {
	bytes, err := json.Marshal(obj)
	if err != nil {
		assert.Fail(t, "Failure marshaling to json", obj)
	}
	return string(bytes)
}
