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
	"encoding/json"
	"fmt"
	"html/template"
	"reflect"
	"sort"
	"strings"

	marin3r "github.com/3scale-ops/marin3r/apis/marin3r/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/contour/dag"
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

		// RuleRedirect indicates whether this rule should be represented
		// in Envoy's config structure as a "redirect", i.e., a request to
		// be responded to immediately by Envoy. Rules fall into one of
		// two families: redirect, and route. Routes are forwarded to
		// upstream clusters and redirects aren't.
		"RuleRedirect": func(rule gatewayv1a2.HTTPRouteRule) bool {
			// If we can find a RequestRedirect filter in the rule then it
			// should be an envoy "redirect".
			for _, filter := range rule.Filters {
				if filter.RequestRedirect != nil {
					return true
				}
			}

			// No RequestRedirect found so it's a "route".
			return false
		},

		// StatusToResponse maps Gateway's numeric statusCode to Envoy's
		// text RedirectResponseCode.
		"StatusToResponse": func(status int) string {
			if status == 301 {
				return "MOVED_PERMANENTLY"
			}

			return "FOUND"
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

// makeGWListeners translates a GWProxy's listeners and routes into
// Envoy Resources.
func makeGWListeners(proxy epicv1.GWProxy, routes []epicv1.GWRoute) (resources []marin3r.EnvoyResource, err error) {
	var (
		referents = []epicv1.GWRoute{}
		raw       = map[gatewayv1a2.Listener][]epicv1.GWRoute{}
		collapsed = map[gatewayv1a2.Listener][]epicv1.GWRoute{}
	)

	// Pre-process Routes before templating them into Listeners.
	for _, listener := range proxy.Spec.Gateway.Listeners {
		referents, err = filterReferentRoutes(listener, routes)
		if err != nil {
			return resources, err
		}
		raw[listener], err = preprocessRoutes(listener, referents)
		if err != nil {
			return resources, err
		}
	}

	// "Collapse" compatible listeners
	collapsed, err = collapse(raw)
	if err != nil {
		return resources, err
	}

	// Process each template.
	for _, template := range proxy.Spec.EnvoyTemplate.EnvoyResources.Listeners {
		for listener, listenerRoutes := range collapsed {
			listenerResource, err := executeListenerTemplate(template.Value, proxy, listener, listenerRoutes)
			if err != nil {
				return resources, err
			}
			resources = append(resources, listenerResource)
		}
	}

	return resources, nil
}

// executeListenerTemplate executes listenerTemplate to generate an
// Envoy Resource.
func executeListenerTemplate(listenerTemplate string, proxy epicv1.GWProxy, listener gatewayv1a2.Listener, routes []epicv1.GWRoute) (marin3r.EnvoyResource, error) {
	var (
		tmpl *template.Template
		err  error
		doc  bytes.Buffer
	)

	// "wash" the Gateway protocol into a core/v1 protocol.
	proto, err := washProtocol(listener.Protocol)
	if err != nil {
		return marin3r.EnvoyResource{}, err
	}

	params := listenerParams{
		ServiceNamespace:  proxy.Namespace,
		ServiceName:       proxy.Name,
		PureLBServiceName: proxy.Spec.DisplayName,
		PortName:          fmt.Sprintf("%s-%d", listener.Protocol, listener.Port),
		Port:              int32(listener.Port),
		Protocol:          proto,
		TotalWeight:       100,
		ClusterWeight:     100,
		Routes:            routes,
	}
	if tmpl, err = template.New("listener").Funcs(funcMap).Parse(listenerTemplate); err != nil {
		return marin3r.EnvoyResource{}, err
	}
	err = tmpl.Execute(&doc, params)
	return marin3r.EnvoyResource{Name: params.PortName, Value: doc.String()}, err
}

// GWProxyToEnvoyConfig translates one of our epicv1.GWproxy resources
// into a Marin3r EnvoyConfig.
func GWProxyToEnvoyConfig(proxy epicv1.GWProxy, routes []epicv1.GWRoute) (marin3r.EnvoyConfig, error) {
	// Build a cluster for each Route back-end reference.
	clusters, err := routesToClusters(proxy, routes)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}

	// Build an Envoy listener for each Gateway listener.
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
				Clusters:  clusters,
				Routes:    proxy.Spec.EnvoyTemplate.EnvoyResources.Routes,
				Listeners: listeners,
				Runtimes:  proxy.Spec.EnvoyTemplate.EnvoyResources.Runtimes,
				Secrets:   proxy.Spec.EnvoyTemplate.EnvoyResources.Secrets,
			},
		},
	}, nil
}

// collapse "collapses" compatible listeners together, i.e.,
// combines them to form a single listener. The input and output are
// maps from listeners to their routes.
// https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.GatewaySpec
func collapse(listeners map[gatewayv1a2.Listener][]epicv1.GWRoute) (map[gatewayv1a2.Listener][]epicv1.GWRoute, error) {
	// Trivial case: if there's zero or one listener then it can't be
	// collapsed.
	if len(listeners) <= 1 {
		return listeners, nil
	}

	collapsed := map[gatewayv1a2.Listener][]epicv1.GWRoute{}

	for l1, r1 := range listeners {
		// Seed the output map with the first listener.
		if len(collapsed) == 0 {
			fmt.Printf("collapsed seeding set with %s\n", l1.Name)
			collapsed[l1] = r1
		} else {
			// For every listener except the first one, compare it to each
			// of the already-collapsed listeners to see if they're
			// compatible.
			for l2 := range collapsed {
				if l1 != l2 {
					if compatibleListeners(l1, l2) {
						fmt.Printf("collapsed collapsing %s into %s\n", l1.Name, l2.Name)
						collapsed[l2] = append(collapsed[l2], listeners[l1]...)
					} else {
						fmt.Printf("collapsed adding %s to set\n", l1.Name)
						collapsed[l1] = r1
					}
				}
			}
		}
	}
	return collapsed, nil
}

// compatibleListeners figures out whether l1 and l2 are "compatible"
// per the rules in
// https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.Gateway
// . If it returns true then l1 and l2 can be "collapsed" into one
// listener.
func compatibleListeners(l1 gatewayv1a2.Listener, l2 gatewayv1a2.Listener) bool {
	// If the ports don't match then they're not compatible.
	if l1.Port != l2.Port {
		fmt.Printf("collapsed %s and %s are NOT compatible: port mismatch\n", l1.Name, l2.Name)
		return false
	}

	// If the two protocols match then they're compatible
	if l1.Protocol == l2.Protocol {
		fmt.Printf("collapsed %s and %s are compatible\n", l1.Name, l2.Name)
		return true
	}

	if (l1.Protocol == gatewayv1a2.HTTPSProtocolType || l1.Protocol == gatewayv1a2.TLSProtocolType) && (l2.Protocol == gatewayv1a2.HTTPSProtocolType || l2.Protocol == gatewayv1a2.TLSProtocolType) {
		fmt.Printf("collapsed %s and %s are compatible\n", l1.Name, l2.Name)
		return true
	}

	fmt.Printf("collapsed %s and %s are NOT compatible\n", l1.Name, l2.Name)
	return false
}

// filterReferentRoutes returns the set of routes that reference
// listener. rawRoutes were chosen because each references at least
// one listener in the Gateway, but it might not be *this*
// listener. We need to eliminate routes that might reference other
// listeners but don't reference this one.
func filterReferentRoutes(listener gatewayv1a2.Listener, rawRoutes []epicv1.GWRoute) (refs []epicv1.GWRoute, err error) {
	refs = []epicv1.GWRoute{}

	for _, route := range rawRoutes {
		if refersToSection(route.Spec.HTTP.ParentRefs, listener) {
			refs = append(refs, route)
		}
	}
	return refs, err
}

// refersToSection returns true if any of the routeRefs refer to the
// listener's section, either by explicit matching or by having an
// empty SectionName.
func refersToSection(routeRefs []gatewayv1a2.ParentReference, listener gatewayv1a2.Listener) bool {
	// Look for any of the route's parentRefs that match. Since we've
	// already checked the reference at the route level we don't need to
	// do that again, but we do need to check the section reference.
	for _, ref := range routeRefs {
		if ref.SectionName == nil || (ref.SectionName != nil && *ref.SectionName == listener.Name) {
			return true
		}
	}

	return false
}

// preprocessRoutes rearranges the routes to make them work in Envoy
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
func preprocessRoutes(listener gatewayv1a2.Listener, rawRoutes []epicv1.GWRoute) ([]epicv1.GWRoute, error) {
	// If the Proxy has a hostname then we'll use that as a default,
	// otherwise we'll use an explicit "all hosts".
	defaultHost := allHosts
	if listener.Hostname != nil {
		defaultHost = *listener.Hostname
	}

	// Make a copy of the matching input routes since we're going to
	// modify some of them.  Any input route with an implicit host
	// (i.e., no hostname) will be made explicit, using
	// defaultHost. This helps below when we group rules by hostname.
	routes := make([]epicv1.GWRoute, len(rawRoutes))
	for i, route := range rawRoutes {
		matchingNames, err := dag.ComputeHosts(route.Spec.HTTP.Hostnames, listener.Hostname)
		if err != nil {
			fmt.Printf("Invalid hostname: %s", err)
		} else {
			if len(matchingNames) == 0 {
				fmt.Println("No matching hostnames")
			} else {
				maybe := rawRoutes[i].DeepCopy()
				maybe.Spec.HTTP.Hostnames = matchingNames
				routes[i] = *maybe
				if len(route.Spec.HTTP.Hostnames) == 0 {
					routes[i].Spec.HTTP.Hostnames = []gatewayv1a2.Hostname{defaultHost}
				}
			}
		}
	}

	// Find the set of unique hostnames in the input routes and create
	// one output Route for each hostname, which is how Envoy likes it.
	hostnames := map[gatewayv1a2.Hostname]*epicv1.GWRoute{}
	for _, route := range routes {
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
	for _, route := range routes {
		for _, hostname := range route.Spec.HTTP.Hostnames {
			hostnames[hostname].Spec.HTTP.Rules = append(hostnames[hostname].Spec.HTTP.Rules, splitMatches(route.Spec.HTTP.Rules)...)
		}
	}

	// Flatten the map into a slice to return to the caller. While we're
	// doing that we sort the rules within each Route so they end up in
	// a Gateway-spec-compliant order (i.e., catch-all match last).
	cooked := []epicv1.GWRoute{}
	for _, route := range hostnames {
		// Combine this Route's rules so Envoy will round-robin among the
		// backends. If we don't do this then we end up with multiple
		// rules and Envoy only uses the first so the others never get
		// reached.
		combined, err := combineRouteRules(*route)
		if err != nil {
			return cooked, err
		}

		// Sort the rules so more complex rules are earlier and simpler
		// rules are later.
		ordered, err := sortRouteRules(combined)
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

// combineRouteRules combines the RouteRules contained in
// unmerged. See shouldMerge() for more info on why we do this. If
// error is non-nil then the returned GWRoute is undefined.
func combineRouteRules(unmerged epicv1.GWRoute) (epicv1.GWRoute, error) {
	// Trivial case: if there are fewer than two rules then we don't
	// need to combine.
	if len(unmerged.Spec.HTTP.Rules) < 2 {
		return unmerged, nil
	}

	// There are at least two rules so we need to combine.

	// Start with a deep copy of the input but with no rules. We'll
	// add/merge each input rule as needed.
	output := unmerged.DeepCopy()
	output.Spec.HTTP.Rules = []gatewayv1a2.HTTPRouteRule{}

	for _, inputRule := range unmerged.Spec.HTTP.Rules {
		addRule := true

		// For each input rule, check it against all of the output rules
		// to see if it can be merged with any of them.
		for i, possibleMerge := range output.Spec.HTTP.Rules {
			if shouldMerge(possibleMerge, inputRule) {
				output.Spec.HTTP.Rules[i].BackendRefs = append(possibleMerge.BackendRefs, inputRule.BackendRefs...)

				bytes, _ := json.MarshalIndent(output.Spec.HTTP.Rules, "", " ")
				fmt.Println("***** Merged: " + string(bytes))

				// Since we merged the inputRule into another output rule, we
				// don't want to add it to the output.
				addRule = false
				break
			}
		}

		if addRule {
			// The input rule wasn't merged so add it as-is to the output.
			output.Spec.HTTP.Rules = append(output.Spec.HTTP.Rules, inputRule)

			bytes, _ := json.MarshalIndent(output.Spec.HTTP.Rules, "", " ")
			fmt.Println("***** Added: " + string(bytes))
		}

	}

	return *output, nil
}

// shouldMerge determines whether or not we should merge two
// RouteRules. This happens when their Matches and Filters are the
// same, in which case we should combine their BackendRefs so Envoy
// will round-robin among them. We need to do this because we might
// have Rules from different Routes which would ordinarily be treated
// separately, but we want to treat them as a single round-robin.
func shouldMerge(route1, route2 gatewayv1a2.HTTPRouteRule) bool {
	// If the Matches are different then we can't merge.
	if !reflect.DeepEqual(route1.Matches, route2.Matches) {
		return false
	}

	// If the Filters are different then we can't merge.
	if !reflect.DeepEqual(route1.Filters, route2.Filters) {
		return false
	}

	// The parts we care about are the same so we can merge.
	return true
}

// isCatchall indicates whether match is the catchall match, i.e., a
// prefix match of "/" with no other criteria. This match needs to be
// the last one since it always matches.
func isCatchall(inputRule gatewayv1a2.HTTPRouteRule) bool {
	// The catchall rule contains only one match so if this one has more
	// or fewer then it's not the catchall
	if len(inputRule.Matches) != 1 {
		return false
	}

	match := inputRule.Matches[0]
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

// washProtocol "washes" proto, optionally upcasing if necessary. If
// error is non-nil then the input protocol wasn't valid.
func washProtocol(proto gatewayv1a2.ProtocolType) (v1.Protocol, error) {
	upper := strings.ToUpper(string(proto))
	if upper == "HTTP" || upper == "HTTPS" || upper == "TLS" {
		upper = "TCP"
	}

	switch upper {
	case "TCP":
		return v1.ProtocolTCP, nil
	case "UDP":
		return v1.ProtocolUDP, nil
	default:
		return v1.ProtocolTCP, fmt.Errorf("Protocol %s is unsupported", proto)
	}
}
