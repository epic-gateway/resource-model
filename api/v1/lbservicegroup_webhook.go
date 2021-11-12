package v1

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	lbsgLog    logr.Logger
	lbsgClient client.Client
)

// SetupWebhookWithManager sets up this webhook to be managed by mgr.
func (lbsg *LBServiceGroup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	lbsgLog = mgr.GetLogger().WithName("lbsg-resource")
	lbsgClient = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).For(lbsg).Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-epic-acnodal-io-v1-lbservicegroup,mutating=false,failurePolicy=fail,groups=epic.acnodal.io,resources=lbservicegroups,versions=v1,name=vlbservicegroup.kb.io,sideEffects=None,admissionReviewVersions=v1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (lbsg *LBServiceGroup) ValidateCreate() error {
	return lbsg.validate()
}

// ValidateUpdate does the same thing that ValidateCreate does.
func (lbsg *LBServiceGroup) ValidateUpdate(old runtime.Object) error {
	return lbsg.validate()
}

// ValidateDelete does nothing and it's never called.
func (r *LBServiceGroup) ValidateDelete() error {
	return nil
}

func (lbsg *LBServiceGroup) validate() error {
	ctx := context.TODO()

	// Block create if there's no owning ServicePrefix
	if _, err := lbsg.getServicePrefix(ctx, lbsgClient); err != nil {
		lbsgLog.Error(err, "rejecting request")
		return fmt.Errorf("error getting owning SP: %s", err.Error())
	}

	// Block create if there's no owning Account
	if _, err := lbsg.getAccount(ctx, lbsgClient); err != nil {
		lbsgLog.Error(err, "rejecting request")
		return fmt.Errorf("error getting owning Account: %s", err.Error())
	}

	return nil
}
