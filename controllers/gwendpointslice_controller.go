package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// GWEndpointSliceReconciler reconciles a GWEndpointSlice object
type GWEndpointSliceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GWEndpointSliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWEndpointSlice{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwendpointslices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwendpointslices/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GWEndpointSlice object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *GWEndpointSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	slice := epicv1.GWEndpointSlice{}

	// Get the resource that caused this event.
	if err := r.Get(ctx, req.NamespacedName, &slice); err != nil {
		l.Info("Can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	if !slice.ObjectMeta.DeletionTimestamp.IsZero() {
		// This rep is marked for deletion. We need to clean up where
		// possible, but also be careful to handle edge cases like the LB
		// being "not found" because it was deleted first.

		l.Info("To be deleted")

		// FIXME: need to find the proxy via routes and deal with routes being deleted
		// // Remove the rep's info from the LB but continue even if there's
		// // an error because we want to *always* remove our finalizer.
		// repInfoErr := removeRepInfo(ctx, r.Client, l, req.NamespacedName.Namespace, slice.Labels[epicv1.OwningProxyLabel], rep)
		var repInfoErr error = nil

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		if err := RemoveFinalizer(ctx, r.Client, &slice, epicv1.FinalizerName); err != nil {
			return done, err
		}

		// Now that we've removed our finalizer we can return and report
		// on any errors that happened during cleanup.
		return done, repInfoErr
	}

	// The proxy is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	if err := AddFinalizer(ctx, r.Client, &slice, epicv1.FinalizerName); err != nil {
		return done, err
	}

	l.Info("Reconciling")

	// Get the proxies that are connected to this slice. It can be more
	// than one since more than one GWRoute and reference this slice,
	// and each GWRoute can reference more than one GWProxy.
	proxies, err := slice.ReferencingProxies(ctx, r.Client)
	if err != nil {
		return done, err
	}

	// Add our endpoints to each of the GWProxies that references us.
	for _, proxy := range proxies {
		l.Info("parent", "proxy", proxy.NamespacedName())

		// Get all of the client-side endpoints for this proxy
		reps, err := proxy.ActiveProxyEndpoints(ctx, r.Client)
		if err != nil {
			return done, err
		}

		// Allocate tunnels
		if err := r.allocateProxyTunnels(ctx, l, proxy, reps); err != nil {
			return done, err
		}
	}

	return done, nil
}

// allocateProxyTunnels adds a tunnel from each endpoints item to each
// node running an Envoy proxy and patches proxy with the tunnel info.
func (r *GWEndpointSliceReconciler) allocateProxyTunnels(ctx context.Context, l logr.Logger, proxy *epicv1.GWProxy, endpoints []epicv1.RemoteEndpoint) error {
	var (
		err        error
		newMap     map[string]epicv1.EPICEndpointMap = map[string]epicv1.EPICEndpointMap{}
		patch      []map[string]interface{}
		patchBytes []byte
		raw        []byte = make([]byte, 8, 8)
	)

	_, _ = rand.Read(raw)

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &epicv1.EPIC{}
	if err := r.Get(ctx, types.NamespacedName{Name: epicv1.ConfigName, Namespace: epicv1.ConfigNamespace}, config); err != nil {
		return err
	}

	// We set up a tunnel from each proxy to each of this slice's nodes.
	for _, clientPod := range endpoints {
		for _, proxyPod := range proxy.Spec.ProxyInterfaces {

			// If we haven't seen this client node yet then initialize its
			// map.
			if _, seenBefore := newMap[clientPod.Spec.NodeAddress]; !seenBefore {
				newMap[clientPod.Spec.NodeAddress] = epicv1.EPICEndpointMap{
					EPICEndpoints: map[string]epicv1.GUETunnelEndpoint{},
				}
			}

			if tunnel, exists := proxy.Spec.GUETunnelMaps[clientPod.Spec.NodeAddress].EPICEndpoints[proxyPod.EPICNodeAddress]; exists {
				// If the tunnel already exists (i.e., two client endpoints in
				// the same service on the same node) then we can just copy
				// the map from the old resource into the patch.
				newMap[clientPod.Spec.NodeAddress].EPICEndpoints[proxyPod.EPICNodeAddress] = tunnel
			} else {
				// This is a new proxyNode/endpointNode pair, so allocate a new
				// tunnel ID for it.
				envoyEndpoint := epicv1.GUETunnelEndpoint{}
				config.Spec.NodeBase.GUEEndpoint.DeepCopyInto(&envoyEndpoint)
				envoyEndpoint.Address = proxyPod.EPICNodeAddress
				if envoyEndpoint.TunnelID, err = allocateTunnelID(ctx, l, r.Client); err != nil {
					return err
				}
				newMap[clientPod.Spec.NodeAddress].EPICEndpoints[proxyPod.EPICNodeAddress] = envoyEndpoint
			}
		}
	}

	// Prepare the patch. It will have two parts: the nudge annotation
	// and the new tunnel map.
	if proxy.Annotations == nil {
		// If this is the first annotation then we need to wrap it in a
		// JSON object
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": map[string]string{"nudge": hex.EncodeToString(raw)},
		}}
	} else {
		// If there are other annotations then we can just add this one
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations/nudge",
			"value": hex.EncodeToString(raw),
		}}
	}
	// If there's already a map then delete it.
	if len(proxy.Spec.GUETunnelMaps) > 0 {
		patch = append(
			patch,
			map[string]interface{}{
				"op":   "remove",
				"path": "/spec/gue-tunnel-endpoints",
			},
		)
	}
	// Add the new tunnel map.
	patch = append(
		patch,
		map[string]interface{}{
			"op":    "add",
			"path":  "/spec/gue-tunnel-endpoints",
			"value": newMap,
		},
	)

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	l.Info(string(patchBytes))
	if err := r.Patch(ctx, proxy, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		l.Error(err, "patching", "proxy", proxy)
		return err
	}
	l.Info("patched", "proxy", proxy)

	return nil
}
