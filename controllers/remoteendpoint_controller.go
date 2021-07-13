package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
)

// RemoteEndpointReconciler reconciles RemoteEndpoint objects.
type RemoteEndpointReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for. In this case the request indicates that an
// endpoint has changed so we need to update the LB's tunnel map.
func (r *RemoteEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("rep", req.NamespacedName)
	rep := epicv1.RemoteEndpoint{}

	// Get the RemoteEndpoint that caused the event
	if err := r.Get(ctx, req.NamespacedName, &rep); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// If the NodeAddress is "" then this is an ad-hoc endpoint, i.e., it doesn't use GUE.
	if rep.Spec.NodeAddress == "" {
		log.Info("ad-hoc, no tunnel setup needed")
		return done, nil
	}

	if !rep.ObjectMeta.DeletionTimestamp.IsZero() {
		// This rep is marked for deletion. We need to clean up where
		// possible, but also be careful to handle edge cases like the LB
		// being "not found" because it was deleted first.

		log.Info("object to be deleted")

		// Remove the rep's info from the LB but continue even if there's
		// an error because we want to *always* remove our finalizer.
		repInfoErr := removeRepInfo(ctx, r.Client, log, req.NamespacedName.Namespace, rep.Labels[epicv1.OwningLoadBalancerLabel], rep.Spec.NodeAddress)

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		if err := RemoveFinalizer(ctx, r.Client, &rep, epicv1.RemoteEndpointFinalizerName); err != nil {
			return done, err
		}

		// Now that we've removed our finalizer we can return and report
		// on any errors that happened during cleanup.
		return done, repInfoErr
	}

	log.Info("reconciling")

	// Get the LB to which this rep belongs.
	lb := epicv1.LoadBalancer{}
	lbname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: rep.Labels[epicv1.OwningLoadBalancerLabel]}
	if err := r.Get(ctx, lbname, &lb); err != nil {
		return done, err
	}

	// The pod is not being deleted, so if it does not have our
	// finalizer, then add it and update the object.
	if err := AddFinalizer(ctx, r.Client, &rep, epicv1.RemoteEndpointFinalizerName); err != nil {
		return done, err
	}

	// Allocate tunnels for this endpoint
	if err := r.addProxyTunnels(ctx, log, &lb, &rep.Spec); err != nil {
		return done, err
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *RemoteEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.RemoteEndpoint{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *RemoteEndpointReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// addProxyTunnels adds a tunnel from ep to each node running an Envoy
// proxy and patches lb with the tunnel info.
func (r *RemoteEndpointReconciler) addProxyTunnels(ctx context.Context, l logr.Logger, lb *epicv1.LoadBalancer, ep *epicv1.RemoteEndpointSpec) error {
	var (
		err            error
		envoyEndpoints map[string]epicv1.GUETunnelEndpoint = map[string]epicv1.GUETunnelEndpoint{}
		patchBytes     []byte
		raw            []byte = make([]byte, 8, 8)
	)

	// prepare a patch to set this rep's tunnel endpoints in the LB
	// status
	_, _ = rand.Read(raw)
	patch := epicv1.LoadBalancer{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				// Sometimes we get a new endpoint that is hosted on a client
				// node that hosts other endpoints that we already knew
				// about. In that case this patch doesn't add anything to the
				// tunnel map so the LB CR doesn't change so the LB agent
				// controller doesn't fire. We add this "nudge" to ensure that
				// the LB agent controller always fires.
				"nudge": hex.EncodeToString(raw),
			},
		},
		Spec: epicv1.LoadBalancerSpec{
			GUETunnelMaps: map[string]epicv1.EPICEndpointMap{
				ep.NodeAddress: {
					EPICEndpoints: envoyEndpoints,
				},
			},
			TrueIngress: lb.Spec.TrueIngress,
		},
	}

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &epicv1.EPIC{}
	if err := r.Get(ctx, types.NamespacedName{Name: epicv1.ConfigName, Namespace: epicv1.ConfigNamespace}, config); err != nil {
		return err
	}

	// We set up a tunnel from each proxy to this endpoint's node.
	for _, proxy := range lb.Spec.ProxyInterfaces {

		// If the tunnel already exists (i.e., two endpoints in the same
		// service on the same node) then we don't need to do anything
		if _, exists := lb.Spec.GUETunnelMaps[ep.NodeAddress].EPICEndpoints[proxy.EPICNodeAddress]; exists {
			envoyEndpoints[proxy.EPICNodeAddress] =
				lb.Spec.GUETunnelMaps[ep.NodeAddress].EPICEndpoints[proxy.EPICNodeAddress]
		} else {
			// This is a new proxyNode/endpointNode pair, so allocate a new
			// tunnel ID for it
			envoyEndpoint := epicv1.GUETunnelEndpoint{}
			config.Spec.NodeBase.GUEEndpoint.DeepCopyInto(&envoyEndpoint)
			envoyEndpoint.Address = proxy.EPICNodeAddress
			if envoyEndpoint.TunnelID, err = allocateTunnelID(ctx, l, r.Client); err != nil {
				return err
			}
			envoyEndpoints[proxy.EPICNodeAddress] = envoyEndpoint
		}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	l.Info(string(patchBytes))
	if err := r.Patch(ctx, lb, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
		l.Info("patching LB status", "lb", lb, "error", err)
		return err
	}
	l.Info("LB status patched", "lb", lb)

	return nil
}

// allocateTunnelID allocates a tunnel ID from the EPIC singleton. If
// this call succeeds (i.e., error is nil) then the returned ID will
// be unique.
func allocateTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {
	tries := 3
	for err = fmt.Errorf(""); err != nil && tries > 0; tries-- {
		tunnelID, err = nextTunnelID(ctx, l, cl)
		if err != nil {
			l.Info("problem allocating tunnel ID", "error", err)
		}
	}
	return tunnelID, err
}

// nextTunnelID gets the next tunnel ID from the EPIC CR by doing a
// read-modify-write cycle. It might be inefficient in terms of not
// using all of the values that it allocates but it's safe because the
// Update() will only succeed if the EPIC hasn't been modified since
// the Get().
//
// This function doesn't retry so if there's a collision with some
// other process the caller needs to retry.
func nextTunnelID(ctx context.Context, l logr.Logger, cl client.Client) (tunnelID uint32, err error) {

	// get the EPIC singleton
	epic := epicv1.EPIC{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: epicv1.ConfigName}, &epic); err != nil {
		l.Info("EPIC get failed", "error", err)
		return 0, err
	}

	// increment the EPIC's tunnelID counter
	epic.Status.CurrentTunnelID++

	l.Info("allocating tunnelID", "epic", epic, "tunnelID", epic.Status.CurrentTunnelID)

	return epic.Status.CurrentTunnelID, cl.Status().Update(ctx, &epic)
}

// removePodInfo removes ep's tunnel info from lbName's GUETunnelMaps.
func removeRepInfo(ctx context.Context, cl client.Client, l logr.Logger, lbNS string, lbName string, epNodeAddr string) error {
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

		if _, hasRep := lb.Spec.GUETunnelMaps[epNodeAddr]; hasRep {
			delete(lb.Spec.GUETunnelMaps, epNodeAddr)

			// Try to update
			return cl.Update(ctx, &lb)
		}

		return nil
	})
}
