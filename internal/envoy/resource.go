// Copyright 2020 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package envoy

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

const (
	allHosts = gatewayv1a2.Hostname("*")
)

var (
	// funcMap provides our template helpers to the golang template
	// code.
	funcMap = template.FuncMap{

		// RefWeightsTotal adds up the weights of the HTTPBackendRefs in
		// the array.
		"RefWeightsTotal": func(refs []gatewayv1a2.HTTPBackendRef) (total int) {
			for _, ref := range refs {
				total += int(*ref.Weight)
			}
			return
		},

		// ToUpper upper-cases a string.
		"ToUpper": func(protocol v1.Protocol) string {
			return strings.ToUpper(string(protocol))
		},

		// PathTypeExact compares the provided matchType to
		// PathMatchExact.
		"PathTypeExact": func(matchType *gatewayv1a2.PathMatchType) bool {
			return *matchType == gatewayv1a2.PathMatchExact
		},

		// PathTypePathPrefix compares the provided matchType to
		// PathMatchPathPrefix.
		"PathTypePathPrefix": func(matchType *gatewayv1a2.PathMatchType) bool {
			return *matchType == gatewayv1a2.PathMatchPathPrefix
		},

		// HeaderTypeExact compares the provided matchType to
		// HeaderMatchExact.
		"HeaderTypeExact": func(matchType *gatewayv1a2.HeaderMatchType) bool {
			return *matchType == gatewayv1a2.HeaderMatchExact
		},

		// HeaderTypeRegex compares the provided matchType to
		// HeaderMatchRegularExpression.
		"HeaderTypeRegex": func(matchType *gatewayv1a2.HeaderMatchType) bool {
			return *matchType == gatewayv1a2.HeaderMatchRegularExpression
		},

		// HostnameOrDefault returns what's passed in if it has at least
		// one hostname. If an empty array is passed in then it returns an
		// array with a single "*" which tells Envoy to match all
		// hostnames.
		"HostnameOrDefault": func(hostnames []gatewayv1a2.Hostname) []gatewayv1a2.Hostname {
			if len(hostnames) > 0 {
				return hostnames
			}
			return []gatewayv1a2.Hostname{"*"}
		},
	}
)

type clusterParams struct {
	ClusterName string
	Endpoints   []epicv1.RemoteEndpoint

	// ServiceNamespace is the namespace of the GWProxy.
	ServiceNamespace string

	// PureLBServiceName is the name of the service on the PureLB side,
	// which is the LB "spec.display-name".
	PureLBServiceName string
	// ServiceName is the name of the Gateway object on the client
	// cluster.
	ServiceName string

	Route epicv1.GWRoute
}
type listenerParams struct {
	Clusters []string
	PortName string
	Port     int32
	Protocol v1.Protocol

	// ServiceNamespace is the namespace of the GWProxy. It's useful
	// when using the "ref" directive to tell Envoy about k8s Secrets.
	ServiceNamespace string

	// These make it easier to assign even weights to sets of more than
	// one cluster. By default the weights need to add up to 100 which
	// is awkward if you have 3 clusters so you can use these values for
	// the total_weight and the per-cluster weight.
	TotalWeight   int
	ClusterWeight int

	// PureLBServiceName is the name of the service on the PureLB side,
	// which is the LB "spec.display-name".
	PureLBServiceName string
	// ServiceName is the name of the GWProxy object, i.e., the Gateway
	// prefixed with the owning LBSG.
	ServiceName string

	Routes []epicv1.GWRoute
}

// routesToClusters translates from our GWRoute objects to Marin3r
// Resources containing text Envoy Cluster configs.
func routesToClusters(proxy epicv1.GWProxy, routes []epicv1.GWRoute) ([]marin3r.EnvoyResource, error) {
	var (
		tmpl     *template.Template
		err      error
		doc      bytes.Buffer
		clusters = []marin3r.EnvoyResource{}
	)

	// Find the set of clusters that are referenced by the routes.
	clusterNames := map[string]struct{}{}
	for _, route := range routes {
		for _, rule := range route.Spec.HTTP.Rules {
			for _, ref := range rule.BackendRefs {
				clusterName := string(ref.Name)
				clusterNames[clusterName] = struct{}{}
			}
		}
	}

	// Process each template
	for _, clusterTemplate := range proxy.Spec.EnvoyTemplate.EnvoyResources.Clusters {
		if tmpl, err = template.New("cluster").Funcs(funcMap).Parse(clusterTemplate.Value); err != nil {
			return clusters, err
		}

		// For each template, process each cluster
		for clusterName := range clusterNames {
			doc.Reset()
			err = tmpl.Execute(&doc, clusterParams{
				ServiceNamespace:  proxy.Namespace,
				ClusterName:       clusterName,
				ServiceName:       proxy.Name,
				PureLBServiceName: proxy.Spec.DisplayName,
			})
			// FIXME: log errors

			clusters = append(clusters, marin3r.EnvoyResource{Name: clusterName, Value: doc.String()})
		}
	}

	return clusters, err
}

// makeGWListeners translates a GWProxy's ports into Envoy Resource
// objects.
func makeGWListeners(service epicv1.GWProxy, routes []epicv1.GWRoute) ([]marin3r.EnvoyResource, error) {
	var (
		resources = []marin3r.EnvoyResource{}
	)

	// Process each template.
	for _, listener := range service.Spec.EnvoyTemplate.EnvoyResources.Listeners {
		for _, port := range service.Spec.PublicPorts {
			listener, err := makeGWListener(listener.Value, service, port, routes)
			if err != nil {
				return resources, err
			}
			resources = append(resources, listener)
		}
	}

	return resources, nil
}

func makeGWListener(listenerConfigFragment string, service epicv1.GWProxy, port v1.ServicePort, routes []epicv1.GWRoute) (marin3r.EnvoyResource, error) {
	var (
		tmpl *template.Template
		err  error
		doc  bytes.Buffer
	)

	params := listenerParams{
		ServiceNamespace:  service.Namespace,
		ServiceName:       service.Name,
		PureLBServiceName: service.Spec.DisplayName,
		PortName:          fmt.Sprintf("%s-%d", port.Protocol, port.Port),
		Port:              port.Port,
		Protocol:          port.Protocol,
		TotalWeight:       100,
		ClusterWeight:     100,
		Routes:            routes,
	}
	if tmpl, err = template.New("listener").Funcs(funcMap).Parse(listenerConfigFragment); err != nil {
		return marin3r.EnvoyResource{}, err
	}
	err = tmpl.Execute(&doc, params)
	return marin3r.EnvoyResource{Name: params.PortName, Value: doc.String()}, err
}

// GWProxyToEnvoyConfigServiceToEnvoyConfig translates one of our
// epicv1.GWproxy resources into a Marin3r EnvoyConfig.
func GWProxyToEnvoyConfig(proxy epicv1.GWProxy, routes []epicv1.GWRoute) (marin3r.EnvoyConfig, error) {
	cluster, err := routesToClusters(proxy, routes)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}
	listeners, err := makeGWListeners(proxy, routes)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}

	return marin3r.EnvoyConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy.Name,
			Namespace: proxy.Namespace,
		},
		Spec: marin3r.EnvoyConfigSpec{
			NodeID:        proxy.Namespace + "." + proxy.Name,
			EnvoyAPI:      proxy.Spec.EnvoyTemplate.EnvoyAPI,
			Serialization: proxy.Spec.EnvoyTemplate.Serialization,
			EnvoyResources: &marin3r.EnvoyResources{
				Endpoints: []marin3r.EnvoyResource{},
				Clusters:  cluster,
				Routes:    proxy.Spec.EnvoyTemplate.EnvoyResources.Routes,
				Listeners: listeners,
				Runtimes:  proxy.Spec.EnvoyTemplate.EnvoyResources.Runtimes,
				Secrets:   proxy.Spec.EnvoyTemplate.EnvoyResources.Secrets,
			},
		},
	}, nil
}

// PreprocessRoutes rearranges the routes to make them work in Envoy
// configurations. The idea is to do the heavy lifting that's better
// suited for Go which allows the templates to be more
// straightforward.
//
// Each Route can have multiple Hostnames but Envoy allows only a
// single hostname per virtual_host, so we need to create one output
// Route for each input hostname.
//
// Each output route contains all of the rules that belong to any
// input route with the same Hostname. Some rules might get duplicated
// because an HTTPRoute can have multiple hostnames but each Envoy
// virtual_host can have only a single hostname. A single output route
// might contain rules from multiple input routes.
func PreprocessRoutes(rawRoutes []epicv1.GWRoute) ([]epicv1.GWRoute, error) {
	// Make any route with an implicit wildcard host (i.e., no hostname)
	// explicit (i.e., "*"). This helps below when we group rules by
	// hostname.
	for i, route := range rawRoutes {
		if len(route.Spec.HTTP.Hostnames) == 0 {
			rawRoutes[i].Spec.HTTP.Hostnames = []gatewayv1a2.Hostname{allHosts}
		}
	}

	// Find the set of unique hostnames in the input routes and create
	// one output Route for each hostname, which is how Envoy likes it.
	hostnames := map[gatewayv1a2.Hostname]*epicv1.GWRoute{}
	for _, route := range rawRoutes {
		for _, hostname := range route.Spec.HTTP.Hostnames {
			hostnames[hostname] = &epicv1.GWRoute{
				Spec: epicv1.GWRouteSpec{
					HTTP: gatewayv1a2.HTTPRouteSpec{
						Hostnames: []gatewayv1a2.Hostname{hostname},
						Rules:     []gatewayv1a2.HTTPRouteRule{},
					},
				},
			}
		}
	}

	// Build each output route by adding the split rules from each input
	// route that contains its hostname.
	for _, route := range rawRoutes {
		for _, hostname := range route.Spec.HTTP.Hostnames {
			hostnames[hostname].Spec.HTTP.Rules = append(hostnames[hostname].Spec.HTTP.Rules, splitMatches(route.Spec.HTTP.Rules)...)
		}
	}

	// Flatten the map into a slice to return to the caller. While we're
	// doing that we sort the rules within each Route so they end up in
	// a Gateway-spec-compliant order (i.e., catch-all match last).
	cooked := []epicv1.GWRoute{}
	for _, route := range hostnames {
		ordered, err := sortRouteRules(*route)
		if err != nil {
			return cooked, err
		}
		cooked = append(cooked, ordered)
	}

	return cooked, nil
}

// sortRouteRules puts the "catch-all" match last so other matches get
// a chance to do their thing. Envoy checks the matches in order so if
// the first one in the list matches the others don't get checked. If
// the first match is the "/" path prefix it will always match.
func sortRouteRules(route epicv1.GWRoute) (epicv1.GWRoute, error) {
	sorted := route.DeepCopy()

	sort.SliceStable(sorted.Spec.HTTP.Rules, func(i, j int) bool {
		iRule := sorted.Spec.HTTP.Rules[i]
		jRule := sorted.Spec.HTTP.Rules[j]

		// Override the order if the first candidate is the catchall path
		// match, i.e. prefix "/" with no other matches.
		if isCatchall(iRule) {
			return false
		}

		// Simpler criteria are easier to match so they need to be checked
		// later than more complex criteria. You might have two criteria
		// with the same path but one of them also has a header. The one
		// with two different matches needs to be first because otherwise
		// it will never even get checked because the simpler one will
		// match in every case that the complex one would.
		//
		// This is a simple approach that probably won't handle every case
		// but will handle many of them.
		if criteriaCount(iRule.Matches[0]) < criteriaCount(jRule.Matches[0]) { // FIXME: handle multi-match rules
			return false
		}

		// We haven't seen anything to cause us to change the order.
		return true
	})

	return *sorted, nil
}

// isCatchall indicates whether match is the catchall match, i.e., a
// prefix match of "/" with no other criteria. This match needs to be
// the last one since it always matches.
func isCatchall(rule gatewayv1a2.HTTPRouteRule) bool {
	// The catchall rule contains only one match so if this one has more
	// or fewer then it's not the catchall
	if len(rule.Matches) != 1 {
		return false
	}

	match := rule.Matches[0]
	if criteriaCount(match) == 1 &&
		match.Path != nil &&
		*match.Path.Type == gatewayv1a2.PathMatchPathPrefix &&
		*match.Path.Value == "/" {
		return true
	}
	return false
}

// criteriaCount returns the count of active criteria in match. This
// can be used when ordering the matches since matches with more
// criteria are more specific and therefore should be checked after
// matches with fewer criteria.
func criteriaCount(match gatewayv1a2.HTTPRouteMatch) (count int) {
	if len(match.Headers) > 0 {
		count++
	}
	if len(match.QueryParams) > 0 {
		count++
	}
	if match.Method != nil {
		count++
	}
	if match.Path != nil {
		count++
	}

	return
}

// splitMatches splits multi-match rules into multiple single-match
// rules. We do this because we need to re-order the matches from
// most-specific to least-specific for Envoy to do its job. A single
// rule might have several matches, but some of the matches might be
// more specific and some might be less specific. In this case some of
// the matches need to be early in the list, and some need to be
// later, but if they all belong to one rule then they end up in the
// same place.
//
// We're able to do this because matches within a rule are "OR-ed",
// i.e., the rule matches if *any* of its matches matches. We can
// therefore take a rule with two matches and turn it into two rules,
// each with one match. This allows us to sort them into different
// places in the list.
func splitMatches(rules []gatewayv1a2.HTTPRouteRule) []gatewayv1a2.HTTPRouteRule {
	splits := []gatewayv1a2.HTTPRouteRule{}

	for _, rule := range rules {
		for _, match := range rule.Matches {
			split := rule.DeepCopy()
			split.Matches = []gatewayv1a2.HTTPRouteMatch{match}
			splits = append(splits, *split)
		}
	}

	return splits
}
