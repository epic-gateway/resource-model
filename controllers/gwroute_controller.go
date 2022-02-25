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

// GWRouteReconciler reconciles a GWRoute object
type GWRouteReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GWRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.GWRoute{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwroutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GWRoute object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *GWRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// read the object that caused the event
	route := epicv1.GWRoute{}
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		l.Info("Can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	if !route.ObjectMeta.DeletionTimestamp.IsZero() {
		// This route is marked for deletion. We need to clean up where
		// possible, but also be careful to handle edge cases.

		l.Info("To be deleted")

		// Remove our finalizer to ensure that we don't block it from
		// being deleted.
		if err := RemoveFinalizer(ctx, r.Client, &route, epicv1.FinalizerName); err != nil {
			return done, err
		}

	} else {

		// The route is not being deleted, so if it does not have our
		// finalizer, then add it and update the object.
		if err := AddFinalizer(ctx, r.Client, &route, epicv1.FinalizerName); err != nil {
			return done, err
		}
	}

	l.Info("Reconciling")

	// This route can reference multiple GWProxies. Nudge each of them.
	for _, parent := range route.Spec.HTTP.ParentRefs {
		proxyName := types.NamespacedName{Namespace: route.Namespace, Name: string(parent.Name)}
		pl := l.WithValues("parent", proxyName)
		pl.Info("Updating")

		proxy := epicv1.GWProxy{}
		if err := r.Get(ctx, proxyName, &proxy); err != nil {
			pl.Info("Can't get parent proxy")
		} else {
			// Nudge the proxy so it updates itself.
			if err := r.nudgeProxy(ctx, pl, &proxy); err != nil {
				return done, err
			}
		}
	}

	return done, nil
}

func (r *GWRouteReconciler) nudgeProxy(ctx context.Context, l logr.Logger, proxy *epicv1.GWProxy) error {
	var (
		err        error
		patch      []map[string]interface{}
		patchBytes []byte
		raw        []byte = make([]byte, 8, 8)
	)

	_, _ = rand.Read(raw)

	// Prepare the patch with the new annotation.
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
