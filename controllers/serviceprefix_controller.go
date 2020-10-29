package controllers

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nettypes "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"
	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
)

// ServicePrefixReconciler reconciles a ServicePrefix object
type ServicePrefixReconciler struct {
	client.Client
	NetClient netclient.K8sCniCncfIoV1Interface
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=egw.acnodal.io,resources=serviceprefixes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=serviceprefixes/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *ServicePrefixReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}
	ctx := context.Background()
	l := r.Log.WithValues("serviceprefix", req.NamespacedName)

	l.Info("fetching")
	netdef := r.fetchNetAttachDef(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name)
	if netdef == nil {
		l.Info("need to create")
		// read the object that caused the event
		sp := &egwv1.ServicePrefix{}
		err = r.Get(ctx, req.NamespacedName, sp)
		if err == nil {

			// create a netdef to work with the ServicePrefix
			netdef, err = r.netdefForSP(sp)
			if err == nil {

				// load the netdef into k8s
				_, err = r.addNetAttachDef(ctx, netdef)
				if err == nil {
					return result, nil

				}
				r.Log.Error(err, "adding netdef")
			} else {
				r.Log.Error(err, "creating netdef")
			}
		} else {
			r.Log.Error(err, "reading ServicePrefix")
		}
	} else {
		l.Info("netdef already exists")
	}

	return result, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *ServicePrefixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// set up a K8sCniCncfIoV1Client which we'll use to create
	// net-attach-defs
	netclient, err := netclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		r.NetClient = nil
		return err
	}
	r.NetClient = netclient

	return ctrl.NewControllerManagedBy(mgr).
		For(&egwv1.ServicePrefix{}).
		Complete(r)
}

// netdefForSG returns a NetworkAttachmentDefinition object that will
// allow Envoy pods to attach to the Multus interface.
func (r *ServicePrefixReconciler) netdefForSP(sp *egwv1.ServicePrefix) (*nettypes.NetworkAttachmentDefinition, error) {
	netdefspec, err := json.Marshal(map[string]interface{}{
		// FIXME: need to add parameters to tell Multus which netdef to use
		"cniVersion": "0.3.1",
		"name":       sp.Name,
		"namespace":  sp.Namespace,
		"plugins": []map[string]interface{}{{
			"type":      "bridge",
			"bridge":    multusInt,
			"isGateway": false,
			"ipam": map[string]interface{}{
				"type":   "host-local",
				"subnet": sp.Spec.Subnet,
			},
		}},
	})
	if err != nil {
		return nil, err
	}

	r.Log.Info("multus config", "config", string(netdefspec))

	return &nettypes.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sp.Name,
			Namespace: sp.Namespace,
		},
		Spec: nettypes.NetworkAttachmentDefinitionSpec{
			Config: string(netdefspec),
		},
	}, nil
}

// addNetAttachDef loads netattach into kubernetes.
func (r *ServicePrefixReconciler) addNetAttachDef(ctx context.Context, netattach *nettypes.NetworkAttachmentDefinition) (*nettypes.NetworkAttachmentDefinition, error) {
	return r.NetClient.
		NetworkAttachmentDefinitions(netattach.ObjectMeta.Namespace).
		Create(ctx, netattach, metav1.CreateOptions{})
}

// fetchNetAttachDef gets the NetworkAttachmentDefinition named "name"
// in the namespace "namespace".
func (r *ServicePrefixReconciler) fetchNetAttachDef(ctx context.Context, namespace, name string) *nettypes.NetworkAttachmentDefinition {
	netdef, err := r.NetClient.
		NetworkAttachmentDefinitions(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	return netdef
}
