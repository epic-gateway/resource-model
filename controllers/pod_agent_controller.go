package controllers

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/pfc"
)

// PodAgentReconciler reconciles a Pod object
type PodAgentReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *PodAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := v1.Pod{}
	result := ctrl.Result{}
	log := r.Log.WithValues("agent-running-on", os.Getenv("EPIC_NODE_NAME"), "pod", req.NamespacedName.Name)

	// read the Pod that caused the event
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		log.Info("can't get pod resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// If it's not an Envoy Pod then do nothing
	if !epicv1.HasEnvoyLabels(pod) {
		log.Info("pod is not a proxy pod")
		return done, nil
	}

	// If it's not running on this node then do nothing
	if os.Getenv("EPIC_NODE_NAME") != pod.Spec.NodeName {
		log.Info("pod is not running on this node", "pod-running-on", pod.Spec.NodeName)
		return done, nil
	}

	// We need this pod's veth info so if the python daemon hasn't
	// filled it in yet then we'll back off and give it more time
	ifIndex, haveIfIndex := pod.ObjectMeta.Annotations[epicv1.IfIndexAnnotation]
	ifName, haveIfName := pod.ObjectMeta.Annotations[epicv1.IfNameAnnotation]
	if !haveIfIndex || !haveIfName {
		log.Info("incomplete proxy interface info, will retry")
		return tryAgain, nil
	}

	log.Info("Reconciling", "ifindex", ifIndex, "ifname", ifName)

	// If this object is *not* being deleted then set up the TrueIngress
	// tagger.
	if pod.ObjectMeta.DeletionTimestamp.IsZero() {
		// add the packet tagger to the Envoy pod veth
		if err := r.configureTagging(log, ifName); err != nil {
			return result, err
		}
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *PodAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *PodAgentReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

func (r *PodAgentReconciler) configureTagging(l logr.Logger, ifname string) error {
	err := pfc.AddQueueDiscipline(ifname)
	if err != nil {
		return err
	}
	return pfc.AddFilter(ifname, "ingress", "tag_rx")
}
