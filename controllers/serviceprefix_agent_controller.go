package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/network"
)

// ServicePrefixAgentReconciler reconciles a ServicePrefix object
type ServicePrefixAgentReconciler struct {
	client.Client
	NetClient     netclient.K8sCniCncfIoV1Interface
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=serviceprefixes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=serviceprefixes/status,verbs=get;update;patch

// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;create

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *ServicePrefixAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}
	log := r.Log.WithValues("serviceprefix", req.NamespacedName)

	// read the object that caused the event
	sp := epicv1.ServicePrefix{}
	err = r.Get(ctx, req.NamespacedName, &sp)
	if err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// configure the Multus bridge interface
	bridge, err := network.ConfigureBridge(log, sp.Spec.MultusBridge, sp.Spec.GatewayAddr())
	if err != nil {
		log.Error(err, "Failed to configure multus bridge")
		return done, err
	}

	// Setup host routing
	subnet, err := sp.Spec.SubnetIPNet()
	if err != nil {
		return done, err
	}
	if err := network.AddRt(subnet, bridge); err != nil {
		log.Error(err, "adding route to multus interface")
		return done, err
	}

	return result, err
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *ServicePrefixAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.ServicePrefix{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *ServicePrefixAgentReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}
