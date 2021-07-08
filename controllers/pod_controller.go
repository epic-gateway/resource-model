package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/prometheus/common/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		log.Info("can't get resource, probably deleted", "pod", req.NamespacedName.Name)
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	log := r.Log.WithValues("pod", req.NamespacedName, "version", pod.ResourceVersion)

	// If it's not an Envoy Pod then do nothing
	if !epicv1.HasEnvoyLabels(pod) {
		log.Info("is not a proxy pod")
		return done, nil
	}

	if !pod.ObjectMeta.DeletionTimestamp.IsZero() {
		// This pod is marked for deletion. We need to clean up where
		// possible, but also be careful to handle edge cases like the LB
		// being "not found" because it was deleted first.

		// Remove the pod's info from the LB but continue even if there's
		// an error because we want to *always* remove our finalizer.
		podInfoErr := removePodInfo(ctx, r.Client, pod.ObjectMeta.Namespace, pod.Labels[epicv1.OwningLoadBalancerLabel], pod.ObjectMeta.Name)

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

	// If the pod doesn't have a hostIP yet then we can't set up tunnels
	if pod.Status.HostIP == "" {
		log.Info("pod has no hostIP yet")
		return tryAgain, nil
	}

	// Get the LB to which this rep belongs
	lb := epicv1.LoadBalancer{}
	lbname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: pod.Labels[epicv1.OwningLoadBalancerLabel]}
	if err := r.Get(ctx, lbname, &lb); err != nil {
		return done, err
	}

	// The pod is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	log.Info("adding finalizer")
	if err := AddFinalizer(ctx, r.Client, &pod, podFinalizerName); err != nil {
		return done, err
	}

	// Allocate tunnels for this pod
	if err := r.addPodTunnels(ctx, log, &lb, &pod); err != nil {
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

// addPodTunnels adds a tunnel from pod to each client node running an
// endpoint and patches lb with the tunnel info.
func (r *PodReconciler) addPodTunnels(ctx context.Context, l logr.Logger, lb *epicv1.LoadBalancer, pod *v1.Pod) error {
	var (
		err        error
		patchBytes []byte
		tunnelMaps map[string]epicv1.EPICEndpointMap = map[string]epicv1.EPICEndpointMap{}
	)

	// prepare a patch to set this rep's tunnel endpoints in the LB
	// status
	patch := epicv1.LoadBalancer{
		Status: epicv1.LoadBalancerStatus{
			GUETunnelMaps: tunnelMaps,
		},
	}

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &epicv1.EPIC{}
	if err := r.Get(ctx, types.NamespacedName{Name: epicv1.ConfigName, Namespace: epicv1.ConfigNamespace}, config); err != nil {
		return err
	}

	// We set up a tunnel from each endpoint to this proxy's node.
	for epAddr, epTunnels := range lb.Status.GUETunnelMaps {

		// If the tunnel doesn't exist (i.e., we haven't seen this
		// epicNode/clientNode pair before) then allocate a new tunnel and
		// add it to the patch.
		if _, exists := epTunnels.EPICEndpoints[pod.Status.HostIP]; !exists {

			// This is a new proxyNode/endpointNode pair, so allocate a new
			// tunnel ID for it
			envoyEndpoint := epicv1.GUETunnelEndpoint{}
			config.Spec.NodeBase.GUEEndpoint.DeepCopyInto(&envoyEndpoint)
			envoyEndpoint.Address = pod.Status.HostIP
			if envoyEndpoint.TunnelID, err = allocateTunnelID(ctx, l, r.Client); err != nil {
				return err
			}
			tunnelMaps[epAddr] = epicv1.EPICEndpointMap{
				EPICEndpoints: map[string]epicv1.GUETunnelEndpoint{
					pod.Status.HostIP: envoyEndpoint,
				},
			}
		}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	l.Info(string(patchBytes))
	if err := r.Status().Patch(ctx, lb, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		l.Info("patching LB status", "lb", lb, "error", err)
		return err
	}
	l.Info("LB status patched", "lb", lb)

	return nil
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

		if podInfo, hasPod := lb.Status.ProxyInterfaces[podName]; hasPod {
			// Remove this pod's entry from the ProxyInterfaces map
			delete(lb.Status.ProxyInterfaces, podName)

			// Find out if any remaining Envoy pods are running on the same
			// node as the one we've deleted.
			hasPods := false
			for _, nodeInfo := range lb.Status.ProxyInterfaces {
				if podInfo.EPICNodeAddress == nodeInfo.EPICNodeAddress {
					hasPods = true
				}
			}

			// If this node has no more Envoy pods then remove this node's
			// entry from any of the EPICEndpointMaps that have it.
			if !hasPods {
				for _, epTunnelMap := range lb.Status.GUETunnelMaps {
					delete(epTunnelMap.EPICEndpoints, podInfo.EPICNodeAddress)
				}
			}

			fmt.Printf("updating %+v\n", lb.Status)

			// Try to update
			return cl.Status().Update(ctx, &lb)
		}

		return nil
	})
}
