package controllers

import (
	"context"
	"strings"

	marin3roperator "github.com/3scale-ops/marin3r/apis/operator.marin3r/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// portsToPorts converts from ServicePorts to ContainerPorts.
func portsToPorts(sPorts []corev1.ServicePort) []marin3roperator.ContainerPort {
	cPorts := make([]marin3roperator.ContainerPort, len(sPorts))

	// Expose the configured ports
	for i, port := range sPorts {
		proto := washProtocol(port.Protocol)
		cPorts[i] = marin3roperator.ContainerPort{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: &proto,
		}
	}

	return cPorts
}

// washProtocol "washes" proto, optionally upcasing if necessary.
func washProtocol(proto corev1.Protocol) corev1.Protocol {
	return corev1.Protocol(strings.ToUpper(string(proto)))
}

func updateDeployment(ctx context.Context, cl client.Client, updated *marin3roperator.EnvoyDeployment) error {
	key := client.ObjectKey{Namespace: updated.GetNamespace(), Name: updated.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing := updated.DeepCopy()

		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, existing); err != nil {
			return err
		}
		updated.Spec.DeepCopyInto(&existing.Spec)

		// Try to update
		return cl.Update(ctx, existing)
	})
}
