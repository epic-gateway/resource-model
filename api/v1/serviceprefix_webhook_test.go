package v1_test

import (
	v1 "epic-gateway.org/resource-model/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Ensure that ServicePrefix implements webhook.Defaulter.
var _ webhook.Defaulter = &v1.ServicePrefix{}
