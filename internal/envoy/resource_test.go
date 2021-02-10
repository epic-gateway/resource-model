package envoy

import (
	"fmt"
	"testing"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	"github.com/stretchr/testify/assert"
	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	clusterConfigSample = `name: {{.ClusterName}}
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
	testService = egwv1.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: egwv1.LoadBalancerSpec{
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
		},
	}
)

func TestServiceToCluster(t *testing.T) {
	cluster, err := ServiceToCluster(testService, []egwv1.RemoteEndpoint{})
	assert.Nil(t, err, "template processing failed")
	fmt.Println(cluster)

	cluster, err = ServiceToCluster(testService, []egwv1.RemoteEndpoint{{
		Spec: egwv1.RemoteEndpointSpec{
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
	listener, err := makeHTTPListener(listenerConfigSample, "test", corev1.ServicePort{
		Protocol: "tcp",
		Port:     42,
	}, "")
	assert.Nil(t, err, "template processing failed")
	fmt.Println(listener)
}