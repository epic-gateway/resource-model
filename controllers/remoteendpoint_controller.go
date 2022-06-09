package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// allocateTunnelID allocates a tunnel ID from the EPIC singleton. If
// this call succeeds (i.e., error is nil) then the returned ID will
// be unique.
func allocateTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {
	tries := 3
	for err = fmt.Errorf(""); err != nil && tries > 0; tries-- {
		tunnelID, err = nextTunnelID(ctx, l, cl)
		if err != nil {
			l.Error(err, "problem allocating tunnel ID")
		}
	}
	return tunnelID, err
}

// nextTunnelID gets the next tunnel ID from the EPIC CR by doing a
// read-modify-write cycle. It might be inefficient in terms of not
// using all of the values that it allocates but it's safe because the
// Update() will only succeed if the EPIC hasn't been modified since
// the Get().
//
// This function doesn't retry so if there's a collision with some
// other process the caller needs to retry.
func nextTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {

	// get the EPIC singleton
	epic := epicv1.EPIC{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: epicv1.ConfigName}, &epic); err != nil {
		l.Error(err, "EPIC get failed")
		return 0, err
	}

	// increment the EPIC's tunnelID counter
	epic.Status.CurrentTunnelID++

	l.Info("allocating tunnelID", "epic", epic, "tunnelID", epic.Status.CurrentTunnelID)

	return epic.Status.CurrentTunnelID, cl.Status().Update(ctx, &epic)
}
