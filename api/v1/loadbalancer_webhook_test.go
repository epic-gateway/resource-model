package v1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

var _ = Describe("LoadbalancerWebhook", func() {
	It("accepts everything", func() {
		lb := v1.LoadBalancer{
			Spec: v1.LoadBalancerSpec{},
		}
		Expect(lb.ValidateCreate()).To(BeNil())
	})
})
