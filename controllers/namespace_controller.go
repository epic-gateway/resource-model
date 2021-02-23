package controllers

import (
	"context"
	"strings"

	marin3r "github.com/3scale/marin3r/apis/operator/v1alpha1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	orgLabels = map[string]string{"app.kubernetes.io/component": "organization", "app.kubernetes.io/part-of": "epic"}
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;get;watch

// +kubebuilder:rbac:groups=operator.marin3r.3scale.net,resources=discoveryservices,verbs=create

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *NamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	result := ctrl.Result{}
	ctx := context.Background()
	l := r.Log.WithValues("namespace", req.NamespacedName.Name)

	// read the object that caused the event
	ns := &v1.Namespace{}
	err = r.Get(ctx, req.NamespacedName, ns)
	if err != nil {
		r.Log.Error(err, "reading Namespace")
		return result, err
	}

	if ns.Status.Phase == v1.NamespaceTerminating {
		l.Info("namespace is Terminating")
		return result, nil
	}

	// Check that the NS has the labels that indicate that it's an EPIC
	// client NameSpace
	if !nsHasLabels(ns, orgLabels) {
		return result, nil
	}

	// Create the Marin3r DiscoveryService that will proxy this
	// namespace's Envoys
	ds := marin3r.DiscoveryService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "discoveryservice",
			Namespace: ns.Name,
		},
		Spec: marin3r.DiscoveryServiceSpec{
			Debug: pointer.BoolPtr(true),
		},
	}
	err = r.Create(ctx, &ds)
	if err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			l.Info("Failed to create new DiscoveryService", "message", err.Error(), "name", ds.Name)
			return result, err
		}
		l.Info("DiscoveryService created previously", "name", ds.Name)
		return result, nil
	}

	l.Info("DiscoveryService created", "name", ds.Name)
	return result, err
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Namespace{}).
		Complete(r)
}

// nsHasLabels indicates whether the provided namespace has the
// provided labels.
//
// FIXME: we should be able to do this with a filter on the
// controller's Watch.
func nsHasLabels(o *v1.Namespace, labels map[string]string) bool {
	for k, v := range labels {
		if !nsHasLabel(o, k, v) {
			return false
		}
	}
	return true
}

func nsHasLabel(o *v1.Namespace, label, value string) bool {
	for k, v := range o.Labels {
		if k == label && v == value {
			return true
		}
	}
	return false
}
