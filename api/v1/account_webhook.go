package v1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var accountlog = logf.Log.WithName("account-resource")

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (r *Account) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(r).Complete()
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=egws,verbs=get;list;watch
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=egws/status,verbs=get;update;patch

// +kubebuilder:webhook:path=/mutate-egw-acnodal-io-v1-account,mutating=true,failurePolicy=fail,groups=egw.acnodal.io,resources=accounts,verbs=create;update,versions=v1,name=maccount.kb.io,webhookVersions=v1beta1,admissionReviewVersions=v1beta1
//
//  FIXME: we use v1beta1 here because controller-runtime doesn't
//  support v1 yet. When it does, we should remove
//  ",webhookVersions=v1beta1,admissionReviewVersions=v1beta1" which
//  will switch to v1 (the default)
//

var _ webhook.Defaulter = &Account{}

// Default sets default values for this Account object.
func (r *Account) Default() {
	var err error
	ctx := context.TODO()

	// add a group ID to this account if necessary
	if r.Spec.GroupID == 0 {
		r.Spec.GroupID, err = allocateGroupID(ctx, crtclient)
		if err != nil {
			accountlog.Info("failed to allocate groupID", "error", err)
		}
	}
}

// allocateGroupID allocates a group ID from the EGW singleton. If
// this call succeeds (i.e., error is nil) then the returned ID will
// be unique.
func allocateGroupID(ctx context.Context, cl client.Client) (groupID uint16, err error) {
	tries := 3
	for err = fmt.Errorf(""); err != nil && tries > 0; tries-- {
		groupID, err = nextGroupID(ctx, cl)
		if err != nil {
			accountlog.Info("problem allocating groupID", "error", err)
		}
	}
	return groupID, err
}

// nextGroupID gets the next account group ID by doing a
// read-modify-write cycle. It might be inefficient in terms of not
// using all of the values that it allocates but it's safe because the
// Update() will only succeed if the EGW hasn't been modified since
// the Get().
//
// This function doesn't retry so if there's a collision with some
// other process the caller needs to retry.
func nextGroupID(ctx context.Context, cl client.Client) (groupID uint16, err error) {

	// get the EGW singleton
	egw := EGW{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: ConfigNamespace, Name: ConfigName}, &egw); err != nil {
		accountlog.Info("EGW get failed", "error", err)
		return 0, err
	}

	// increment the allocator
	egw.Status.CurrentGroupID++

	accountlog.Info("allocating groupID", "egw", egw)

	return egw.Status.CurrentGroupID, cl.Status().Update(ctx, &egw)
}
