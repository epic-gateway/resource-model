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

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

var (
	funcMap = template.FuncMap{
		"ToUpper": toUpper,
	}
)

type clusterParams struct {
	ClusterName string
	Endpoints   []epicv1.RemoteEndpoint

	// PureLBServiceName is the name of the service on the PureLB side,
	// which is the LB "spec.display-name".
	PureLBServiceName string
	// ServiceName is the name of the LoadBalancer object, i.e., a the
	// PureLB service prefixed with the owning LBSG.
	ServiceName string
}
type listenerParams struct {
	Clusters []string
	PortName string
	Port     int32
	Protocol v1.Protocol

	// These make it easier to assign even weights to sets of more than
	// one cluster. By default the weights need to add up to 100 which
	// is awkward if you have 3 clusters so you can use these values for
	// the total_weight and the per-cluster weight.
	TotalWeight   int
	ClusterWeight int

	// PureLBServiceName is the name of the service on the PureLB side,
	// which is the LB "spec.display-name".
	PureLBServiceName string
	// ServiceName is the name of the LoadBalancer object, i.e., a the
	// PureLB service prefixed with the owning LBSG.
	ServiceName string
}

// ServiceToCluster translates from our RemoteEndpoint objects to a
// Marin3r Resource containing a text Envoy Cluster config.
func ServiceToCluster(service epicv1.LoadBalancer, endpoints []epicv1.RemoteEndpoint) ([]marin3r.EnvoyResource, error) {
	var (
		tmpl     *template.Template
		err      error
		doc      bytes.Buffer
		clusters = []marin3r.EnvoyResource{}
	)

	// Process each template
	for _, clusterTemplate := range service.Spec.EnvoyTemplate.EnvoyResources.Clusters {
		if tmpl, err = template.New("cluster").Funcs(funcMap).Parse(clusterTemplate.Value); err != nil {
			return []marin3r.EnvoyResource{}, err
		}

		// For each template, process each cluster
		for _, clusterName := range service.Spec.UpstreamClusters {
			doc.Reset()
			err = tmpl.Execute(&doc, clusterParams{
				ClusterName:       clusterName,
				ServiceName:       service.Name,
				PureLBServiceName: service.Spec.DisplayName,
				Endpoints:         endpoints,
			})

			clusters = append(clusters, marin3r.EnvoyResource{Name: clusterName, Value: doc.String()})
		}
	}

	return clusters, err
}

// makeHTTPListeners translates an epicv1.LoadBalancer's ports into
// Envoy Listener objects.
func makeHTTPListeners(service epicv1.LoadBalancer) ([]marin3r.EnvoyResource, error) {
	var (
		resources = []marin3r.EnvoyResource{}
	)

	// Process each template, but only if we have at least one cluster
	if len(service.Spec.UpstreamClusters) > 0 {
		for _, listener := range service.Spec.EnvoyTemplate.EnvoyResources.Listeners {
			for _, port := range service.Spec.PublicPorts {
				listener, err := makeHTTPListener(listener.Value, service, port)
				if err != nil {
					return resources, err
				}
				resources = append(resources, listener)
			}
		}
	}

	return resources, nil
}

func makeHTTPListener(listenerConfigFragment string, service epicv1.LoadBalancer, port v1.ServicePort) (marin3r.EnvoyResource, error) {
	var (
		tmpl *template.Template
		err  error
		doc  bytes.Buffer
	)

	// We want the template to be able to range over the clusters but we
	// might not have any so make sure that we pass in at least an empty
	// array.
	clusters := service.Spec.UpstreamClusters
	if clusters == nil {
		clusters = []string{}
	}

	params := listenerParams{
		ServiceName:       service.Name,
		PureLBServiceName: service.Spec.DisplayName,
		Clusters:          clusters,
		PortName:          fmt.Sprintf("%s-%d", port.Protocol, port.Port),
		Port:              port.Port,
		Protocol:          port.Protocol,
		TotalWeight:       (100 / len(clusters)) * len(clusters),
		ClusterWeight:     100 / len(clusters),
	}
	if tmpl, err = template.New("listener").Funcs(funcMap).Parse(listenerConfigFragment); err != nil {
		return marin3r.EnvoyResource{}, err
	}
	err = tmpl.Execute(&doc, params)
	return marin3r.EnvoyResource{Name: params.PortName, Value: doc.String()}, err
}

// ServiceToEnvoyConfig translates one of our epicv1.LoadBalancers into
// a Marin3r EnvoyConfig
func ServiceToEnvoyConfig(service epicv1.LoadBalancer, endpoints []epicv1.RemoteEndpoint) (marin3r.EnvoyConfig, error) {
	cluster, err := ServiceToCluster(service, endpoints)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}
	listeners, err := makeHTTPListeners(service)
	if err != nil {
		return marin3r.EnvoyConfig{}, err
	}

	return marin3r.EnvoyConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
		},
		Spec: marin3r.EnvoyConfigSpec{
			NodeID:        service.Namespace + "." + service.Name,
			EnvoyAPI:      service.Spec.EnvoyTemplate.EnvoyAPI,
			Serialization: service.Spec.EnvoyTemplate.Serialization,
			EnvoyResources: &marin3r.EnvoyResources{
				Endpoints: []marin3r.EnvoyResource{},
				Clusters:  cluster,
				Routes:    service.Spec.EnvoyTemplate.EnvoyResources.Routes,
				Listeners: listeners,
				Runtimes:  service.Spec.EnvoyTemplate.EnvoyResources.Runtimes,
				Secrets:   service.Spec.EnvoyTemplate.EnvoyResources.Secrets,
			},
		},
	}, nil
}

func toUpper(protocol v1.Protocol) string {
	return strings.ToUpper(string(protocol))
}
