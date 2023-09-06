package v1_test

import (
	v1 "epic-gateway.org/resource-model/api/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GWProxyWebhook", func() {
	It("accepts everything", func() {
		lb := v1.GWProxy{
			Spec: v1.GWProxySpec{},
		}
		// no owning SG label, this should fail
		Expect(lb.ValidateCreate()).ToNot(BeNil())
	})
})
