package v1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("LoadbalancerWebhook", func() {
	It("accepts LBs with unique endpoints and rejects ones with duplicates", func() {
		// start with an LB whose endpoints are unique
		lb := v1.LoadBalancer{
			Spec: v1.LoadBalancerSpec{
				Endpoints: []v1.LoadBalancerEndpoint{
					{
						Address: "1.1.1.1",
						Port:    corev1.EndpointPort{Protocol: corev1.ProtocolTCP, Port: 42},
					},
					{
						Address: "1.1.1.2",
						Port:    corev1.EndpointPort{Protocol: corev1.ProtocolTCP, Port: 42},
					},
					{
						Address: "1.1.1.1",
						Port:    corev1.EndpointPort{Protocol: corev1.ProtocolTCP, Port: 27},
					},
					{
						Address: "1.1.1.1",
						Port:    corev1.EndpointPort{Protocol: corev1.ProtocolUDP, Port: 42},
					},
				},
			},
		}
		Expect(lb.ValidateCreate()).To(BeNil())

		// add an EP that's the same as one of the existing ones
		lb.Spec.Endpoints = append(lb.Spec.Endpoints, v1.LoadBalancerEndpoint{
			Address: "1.1.1.1",
			Port:    corev1.EndpointPort{Protocol: corev1.ProtocolTCP, Port: 42},
		})
		Expect(lb.ValidateCreate()).To(Not(BeNil()))
	})
})
