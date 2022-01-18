package v1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "gitlab.com/acnodal/epic/resource-model/api/v1"
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
