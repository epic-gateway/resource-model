module gitlab.com/acnodal/epic/resource-model

go 1.16

replace (
	github.com/3scale-ops/marin3r => gitlab.com/acnodal/epic/marin3r v0.9.1-epic7
	sigs.k8s.io/gateway-api => github.com/purelb/gateway-api v0.5.0-rc1-epic0
)

require (
	github.com/3scale-ops/marin3r v0.9.1
	github.com/containernetworking/plugins v0.8.7
	github.com/go-logr/logr v1.2.0
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200626054723-37f83d1996bc
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.18.1
	github.com/prometheus/client_golang v1.12.1
	github.com/spf13/cobra v1.4.0
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0
	gitlab.com/acnodal/packet-forwarding-component/src/go v0.0.0-20201020212529-ed4982208c08
	k8s.io/api v0.24.1
	k8s.io/apimachinery v0.24.1
	k8s.io/client-go v0.24.1
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/controller-runtime v0.12.1
	sigs.k8s.io/gateway-api v0.5.0-rc1
)
