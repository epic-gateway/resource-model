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
	"strings"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

const (
	clusterConfigFragment = `name: {{.ClusterName}}
connect_timeout: 2s
type: STRICT_DNS
lb_policy: ROUND_ROBIN
load_assignment:
  cluster_name: {{.ClusterName}}
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
	listenerConfigFragment = `name: {{.PortName}}
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
        stat_prefix: ingress_http
        route_config:
          name: local_route
          virtual_hosts:
            - name: {{.ClusterName}}
              domains: ["*"]
              routes:
                - match:
                    prefix: "/"
                  route:
                    cluster: {{.ClusterName}}
        http_filters:
          - name: envoy.router
`
)

var (
	funcMap = template.FuncMap{
		"ToUpper": toUpper,
	}
)

type clusterParams struct {
	ClusterName string
	Endpoints   []egwv1.RemoteEndpoint
}
type listenerParams struct {
	ClusterName string
	PortName    string
	Port        int32
	Protocol    v1.Protocol
}

// ServiceToCluster translates from our RemoteEndpoint objects to a
// Marin3r Resource containing a text Envoy Cluster config.
func ServiceToCluster(name string, endpoints []egwv1.RemoteEndpoint) ([]marin3r.EnvoyResource, error) {
	var (
		tmpl     *template.Template
		err      error
		doc      bytes.Buffer
		fullName string = name + "Upstream"
	)

	params := clusterParams{
		ClusterName: fullName,
		Endpoints:   endpoints,
	}
	if tmpl, err = template.New("cluster").Funcs(funcMap).Parse(clusterConfigFragment); err != nil {
		return []marin3r.EnvoyResource{}, err
	}
	err = tmpl.Execute(&doc, params)
	return []marin3r.EnvoyResource{{Name: params.ClusterName, Value: doc.String()}}, err
}

// makeHTTPListeners translates an egwv1.LoadBalancer's ports into
// Envoy Listener objects.
func makeHTTPListeners(service egwv1.LoadBalancer, upstreamHost string) ([]marin3r.EnvoyResource, error) {
	var (
		resources = []marin3r.EnvoyResource{}
	)

	for _, port := range service.Spec.PublicPorts {
		listener, err := makeHTTPListener(service.Name, port, upstreamHost)
		if err != nil {
			return resources, err
		}
		resources = append(resources, listener)
	}

	return resources, nil
}

func makeHTTPListener(serviceName string, port v1.ServicePort, upstreamHost string) (marin3r.EnvoyResource, error) {
	var (
		tmpl        *template.Template
		err         error
		doc         bytes.Buffer
		clusterName string = serviceName + "Upstream"
	)
	params := listenerParams{
		ClusterName: clusterName,
		PortName:    fmt.Sprintf("%s-%d", port.Protocol, port.Port),
		Port:        port.Port,
		Protocol:    port.Protocol,
	}
	if tmpl, err = template.New("listener").Funcs(funcMap).Parse(listenerConfigFragment); err != nil {
		return marin3r.EnvoyResource{}, err
	}
	err = tmpl.Execute(&doc, params)
	return marin3r.EnvoyResource{Name: params.PortName, Value: doc.String()}, err
}

// ServiceToEnvoyConfig translates one of our egwv1.LoadBalancers into
// a Marin3r EnvoyConfig
func ServiceToEnvoyConfig(service egwv1.LoadBalancer, endpoints []egwv1.RemoteEndpoint) (marin3r.EnvoyConfig, error) {
	var serialization = "yaml"

	cluster, err := ServiceToCluster(service.Name, endpoints)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}
	listeners, err := makeHTTPListeners(service, service.Spec.PublicAddress)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}
	return marin3r.EnvoyConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
		Spec: marin3r.EnvoyConfigSpec{
			Serialization: &serialization,
			NodeID:        service.Namespace + "." + service.Name,
			EnvoyResources: &marin3r.EnvoyResources{
				Endpoints: []marin3r.EnvoyResource{},
				Clusters:  cluster,
				Listeners: listeners,
			},
		},
	}, nil
}

func toUpper(protocol v1.Protocol) string {
	return strings.ToUpper(string(protocol))
}
