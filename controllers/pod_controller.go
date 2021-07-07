package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

const (
	podFinalizerName = "pod.controller-manager.epic.acnodal.io/finalizer"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pod := v1.Pod{}
	log := r.Log.WithValues("pod", req.NamespacedName.Name)

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// If it's not an Envoy Pod then do nothing
	if !epicv1.HasEnvoyLabels(pod) {
		log.Info("is not a proxy pod")
		return done, nil
	}

	log.Info("Reconciling")

	if !pod.ObjectMeta.DeletionTimestamp.IsZero() {
		// The call to HasEnvoyLabels() above validated that the pod has
		// OwningLoadBalancerLabel in its labels.
		lbName, _ := pod.ObjectMeta.Labels[epicv1.OwningLoadBalancerLabel]

		// Remove the pod's info from the LB but continue even if there's
		// an error because we want to *always* remove our finalizer.
		podInfoErr := removePodInfo(ctx, r.Client, pod.ObjectMeta.Namespace, lbName, pod.ObjectMeta.Name)

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		log.Info("removing finalizer")
		if err := RemoveFinalizer(ctx, r.Client, &pod, podFinalizerName); err != nil {
			return done, err
		}

		// Now that we've removed our finalizer we can return and report
		// on any errors that happened during cleanup.
		return done, podInfoErr
	}

	// The pod is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	log.Info("adding finalizer")
	if err := AddFinalizer(ctx, r.Client, &pod, podFinalizerName); err != nil {
		return done, err
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *PodReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// removePodInfo removes podName's info from lbName's ProxyInterfaces
// map.
func removePodInfo(ctx context.Context, cl client.Client, lbNS string, lbName string, podName string) error {
	var lb epicv1.LoadBalancer

	key := client.ObjectKey{Namespace: lbNS, Name: lbName}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, &lb); err != nil {
			return err
		}

		if _, hasPod := lb.Status.ProxyInterfaces[podName]; hasPod {
			delete(lb.Status.ProxyInterfaces, podName)

			fmt.Printf("updating %+v\n", lb.Status.ProxyInterfaces)

			// Try to update
			return cl.Status().Update(ctx, &lb)
		}

		return nil
	})
}