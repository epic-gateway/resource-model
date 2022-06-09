package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;update;patch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	var podInfoErr error = nil
	pod := v1.Pod{}

	// read the object that caused the event
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		l.Info("can't get resource, probably deleted", "pod", req.NamespacedName.Name)
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	l = l.WithValues("version", pod.ResourceVersion)

	// If it's not an Envoy Pod then do nothing
	if !epicv1.HasEnvoyLabels(pod) {
		l.Info("is not a proxy pod")
		return done, nil
	}

	if !pod.ObjectMeta.DeletionTimestamp.IsZero() {
		// This pod is marked for deletion. We need to clean up where
		// possible, but also be careful to handle edge cases like the
		// owner being "not found" because it was deleted first.

		// Remove the pod's info from the proxy but continue even if
		// there's an error because we want to *always* remove our
		// finalizer.
		owningProxy, belongsToProxy := pod.Labels[epicv1.OwningProxyLabel]
		if belongsToProxy {
			podInfoErr = epicv1.RemoveProxyInfo(ctx, r.Client, pod.ObjectMeta.Namespace, owningProxy, pod.ObjectMeta.Name)
		}

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		l.Info("removing finalizer")
		if err := RemoveFinalizer(ctx, r.Client, &pod, epicv1.FinalizerName); err != nil {
			return done, err
		}

		// Now that we've removed our finalizer we can return and report
		// on any errors that happened during cleanup.
		return done, podInfoErr
	}

	// The pod is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	l.Info("adding finalizer")
	if err := AddFinalizer(ctx, r.Client, &pod, epicv1.FinalizerName); err != nil {
		return done, err
	}

	// If the pod doesn't have a hostIP yet then we can't set up tunnels
	if pod.Status.HostIP == "" {
		l.Info("pod has no hostIP yet")
		return tryAgain, nil
	}

	owningProxy, belongsToProxy := pod.Labels[epicv1.OwningProxyLabel]
	if belongsToProxy {
		// Get the Proxy to which this rep belongs
		proxy := epicv1.GWProxy{}
		proxyName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: owningProxy}
		if err := r.Get(ctx, proxyName, &proxy); err != nil {
			return done, err
		}

		// Allocate tunnels for this pod
		if err := r.addProxyTunnels(ctx, l, &proxy, &pod); err != nil {
			return done, err
		}

		return done, nil
	}

	return done, fmt.Errorf("pod %s isn't owned by either an LB or a GWP", pod.Name)
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

// addProxyTunnels adds a tunnel from proxy to each client node running an
// endpoint and patches lb with the tunnel info.
func (r *PodReconciler) addProxyTunnels(ctx context.Context, l logr.Logger, proxy *epicv1.GWProxy, pod *v1.Pod) error {
	var (
		err        error
		patchBytes []byte
		tunnelMaps map[string]epicv1.EPICEndpointMap = map[string]epicv1.EPICEndpointMap{}
	)

	// prepare a patch to set this rep's tunnel endpoints in the proxy
	// status
	patch := epicv1.GWProxy{
		Spec: epicv1.GWProxySpec{
			GUETunnelMaps: tunnelMaps,
		},
	}

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &epicv1.EPIC{}
	if err := r.Get(ctx, types.NamespacedName{Name: epicv1.ConfigName, Namespace: epicv1.ConfigNamespace}, config); err != nil {
		return err
	}

	// We set up a tunnel from each client-side node to this proxy's
	// node.
	for epAddr, epTunnels := range proxy.Spec.GUETunnelMaps {

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
	if err := r.Patch(ctx, proxy, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		l.Error(err, "patching tunnel maps", "proxy", proxy)
		return err
	}
	l.Info("tunnel maps patched", "proxy", proxy)

	return nil
}
