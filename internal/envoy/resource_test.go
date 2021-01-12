package envoy

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestServiceToCluster(t *testing.T) {
	cluster, err := ServiceToCluster("test", []v1.RemoteEndpoint{})
	assert.Nil(t, err, "template processing failed")
	fmt.Println(cluster)

	cluster, err = ServiceToCluster("test", []v1.RemoteEndpoint{{
		Spec: v1.RemoteEndpointSpec{
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
	listener, err := makeHTTPListener("test", corev1.ServicePort{
		Protocol: "tcp",
		Port:     42,
	}, "")
	assert.Nil(t, err, "template processing failed")
	fmt.Println(listener)
}
