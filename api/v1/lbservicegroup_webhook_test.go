package v1_test

import (
	v1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Ensure that LBServiceGroup implements webhook.Validator.
var _ webhook.Validator = &v1.LBServiceGroup{}
