package v1_test

import (
	v1 "epic-gateway.org/resource-model/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadbalancerWebhook", func() {
	It("accepts everything", func() {
		lb := v1.LoadBalancer{
			Spec: v1.LoadBalancerSpec{},
		}
		// no owning SG label, this should fail
		Expect(lb.ValidateCreate()).ToNot(BeNil())
	})
})
