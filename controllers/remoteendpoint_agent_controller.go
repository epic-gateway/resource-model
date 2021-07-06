package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	epicexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
)

// RemoteEndpointAgentReconciler reconciles RemoteEndpoint objects.
type RemoteEndpointAgentReconciler struct {
	client.Client
	Log           logr.Logger
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=remoteendpoints/status,verbs=get;update;patch

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *RemoteEndpointAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("rep", req.NamespacedName)
	rep := &epicv1.RemoteEndpoint{}
	lb := &epicv1.LoadBalancer{}
	sg := &epicv1.LBServiceGroup{}
	account := &epicv1.Account{}

	// Get the RemoteEndpoint that caused the event
	if err := r.Get(ctx, req.NamespacedName, rep); err != nil {
		log.Info("can't get resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// If the NodeAddress is "" then this is an ad-hoc endpoint, i.e., it doesn't use GUE.
	if rep.Spec.NodeAddress == "" {
		log.Info("ad-hoc, no per-node setup needed")
		return done, nil
	}

	log.Info("reconciling")

	// Check if k8s wants to delete this RemoteEndpoint
	if !rep.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("object to be deleted")

		if controllerutil.ContainsFinalizer(rep, epicv1.RemoteEndpointAgentFinalizerName) {
			// remove our finalizer from the list and update the object
			controllerutil.RemoveFinalizer(rep, epicv1.RemoteEndpointAgentFinalizerName)
			if err := r.Update(ctx, rep); err != nil {
				return done, err
			}
		}

		// Attempt to clean up the PFC but continue even if it fails
		if err := cleanup(log, rep.Spec, rep.Status.ProxyIfindex, rep.Status.TunnelID, rep.Status.GroupID, rep.Status.ServiceID); err != nil {
			log.Error(err, "Failed to cleanup PFC")
		}

	} else {

		// Get the "stack" of CRs to which this rep belongs: LoadBalancer,
		// LBServiceGroup, and Account. They provide configuration data
		// that we need but that isn't contained in the rep.
		lbname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: rep.Labels[epicv1.OwningLoadBalancerLabel]}
		if err := r.Get(ctx, lbname, lb); err != nil {
			return done, err
		}

		sgName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Labels[epicv1.OwningLBServiceGroupLabel]}
		if err := r.Get(ctx, sgName, sg); err != nil {
			return done, err
		}

		accountName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: sg.Labels[epicv1.OwningAccountLabel]}
		if err := r.Get(ctx, accountName, account); err != nil {
			return done, err
		}

		log.Info("setting up tunnels")

		// If any of the Envoy proxy pods need to have their interface
		// data set by the Python daemon then back off and retry later
		if len(lb.Status.ProxyInterfaces) < 1 {
			log.Info("proxy interface info missing")
			return tryAgain, nil
		}
		for proxyName, proxyInfo := range lb.Status.ProxyInterfaces {
			if proxyInfo.Name == "" {
				log.Info("proxy interface info incomplete", "name", proxyName)
				return tryAgain, nil
			}
		}

		// The endpoint isn't being deleted, so set up a tunnel to it from
		// each proxy pod on this node.
		for proxyName, proxyInfo := range lb.Status.ProxyInterfaces {

			log = r.Log.WithValues("proxy", proxyName)

			// Skip if the relevant Envoy isn't running on this host. We can
			// tell because there's an entry in the ProxyInterfaces map but
			// it's not for this host.
			if proxyInfo.EPICNodeAddress != os.Getenv("EPIC_HOST") {
				log.Info("Not me: status has no proxy interface info for this host")
				continue
			}
			log.Info("LB status veth info", "ifindex", proxyInfo.Index, "ifname", proxyInfo.Name)

			// Add GUE ingress address/port to the endpoint
			gueEp, err := r.setGUEIngressAddress(ctx, log, lb, &rep.Spec)
			if err != nil {
				log.Error(err, "Patching LB status")
				return done, err
			}

			// configure the GUE tunnel
			if err := configureTunnel(log, gueEp); err != nil {
				log.Error(err, "configuring GUE tunnel")
				return done, err
			}

			// configure the GUE "service"
			if err := configureService(log, rep.Spec, proxyInfo.Index, account.Spec.GroupID, lb.Spec.ServiceID, gueEp.TunnelID, lb.Spec.TunnelKey); err != nil {
				log.Error(err, "configuring GUE service")
				return done, err
			}

			// Cache some values that we might need later if we need to delete
			// the endpoint
			patchBytes, err := json.Marshal(
				epicv1.RemoteEndpoint{
					Status: epicv1.RemoteEndpointStatus{
						GroupID:      account.Spec.GroupID,
						ServiceID:    lb.Spec.ServiceID,
						ProxyIfindex: proxyInfo.Index,
						TunnelID:     gueEp.TunnelID}})
			if err != nil {
				log.Error(err, "marshaling EP patch", "ep", rep)
				return done, err
			}
			if err := r.Status().Patch(ctx, rep, client.RawPatch(types.MergePatchType, patchBytes)); err != nil {
				log.Error(err, "patching EP status", "ep", rep)
				return done, err
			}
		}
	}

	return done, nil
}

// SetupWithManager sets up this controller to work with the mgr.
func (r *RemoteEndpointAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicv1.RemoteEndpoint{}).
		Complete(r)
}

// Scheme returns this reconciler's scheme.
func (r *RemoteEndpointAgentReconciler) Scheme() *runtime.Scheme {
	return r.RuntimeScheme
}

// setGUEIngressAddress sets the GUEAddress/Port fields of the
// LoadBalancer status. This is used by PureLB to open a GUE tunnel
// back to the EPIC. It returns a GUETunnelRemoteEndpoint, either the newly
// created one or the existing one. If error is non-nil then the
// GUETunnelRemoteEndpoint is invalid.
func (r *RemoteEndpointAgentReconciler) setGUEIngressAddress(ctx context.Context, l logr.Logger, lb *epicv1.LoadBalancer, ep *epicv1.RemoteEndpointSpec) (epicv1.GUETunnelEndpoint, error) {
	var (
		err           error
		purelbEPMap   epicv1.EPICEndpointMap
		envoyEndpoint epicv1.GUETunnelEndpoint
		exists        bool
	)

	// We set up a tunnel to each client node that has an endpoint that
	// belongs to this service. If we already have a tunnel to this
	// endpoint's node (i.e., two endpoints in the same service on the
	// same node) then we don't need to do anything
	if purelbEPMap, exists = lb.Status.GUETunnelMaps[ep.NodeAddress]; exists {
		if envoyEndpoint, exists = purelbEPMap.EPICEndpoints[os.Getenv("EPIC_HOST")]; exists {
			l.Info("Envoy/PureLB node pair already has a tunnel", "endpoint", ep)
			return envoyEndpoint, nil
		}
	}

	// fetch the node config; it tells us the GUEEndpoint for this node
	config := &epicv1.EPIC{}
	err = r.Get(ctx, types.NamespacedName{Name: epicv1.ConfigName, Namespace: epicv1.ConfigNamespace}, config)
	if err != nil {
		return envoyEndpoint, err
	}

	config.Spec.NodeBase.GUEEndpoint.DeepCopyInto(&envoyEndpoint)
	envoyEndpoint.Address = os.Getenv("EPIC_HOST")
	envoyEndpoint.TunnelID, err = allocateTunnelID(ctx, l, r.Client)
	if err != nil {
		return envoyEndpoint, err
	}

	// prepare a patch to set this node's tunnel endpoint in the LB status
	patch := epicv1.LoadBalancer{
		Status: epicv1.LoadBalancerStatus{
			GUETunnelMaps: map[string]epicv1.EPICEndpointMap{
				ep.NodeAddress: {
					EPICEndpoints: map[string]epicv1.GUETunnelEndpoint{
						os.Getenv("EPIC_HOST"): envoyEndpoint,
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return envoyEndpoint, err
	}
	l.Info(string(patchBytes))

	// apply the patch
	err = r.Status().Patch(ctx, lb, client.RawPatch(types.MergePatchType, patchBytes))
	if err != nil {
		l.Info("patching LB status", "lb", lb, "error", err)
		return envoyEndpoint, err
	}

	l.Info("LB status patched", "lb", lb)
	return envoyEndpoint, err
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

func configureTunnel(l logr.Logger, ep epicv1.GUETunnelEndpoint) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel get %[1]d || /opt/acnodal/bin/cli_tunnel set %[1]d %[2]s %[3]d 0 0", ep.TunnelID, ep.Address, ep.Port.Port)
	return epicexec.RunScript(l, script)
}

func configureService(l logr.Logger, ep epicv1.RemoteEndpointSpec, ifindex int, groupID uint16, serviceID uint16, tunnelID uint32, tunnelAuth string) error {
	script := fmt.Sprintf("/opt/acnodal/bin/cli_service set-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, tunnelAuth, tunnelID, ep.Address, ep.Port.Port, ifindex)
	return epicexec.RunScript(l, script)
}

// cleanup undoes the PFC setup that we did for this RemoteEndpoint.
func cleanup(l logr.Logger, ep epicv1.RemoteEndpointSpec, ifindex int, tunnelID uint32, groupID uint16, serviceID uint16) error {
	script1 := fmt.Sprintf("/opt/acnodal/bin/cli_service del-gw %[1]d %[2]d %[3]s %[4]d tcp %[5]s %[6]d %[7]d", groupID, serviceID, "unused", tunnelID, ep.Address, ep.Port.Port, ifindex)
	script2 := fmt.Sprintf("/opt/acnodal/bin/cli_tunnel del %[1]d", tunnelID)
	err1 := epicexec.RunScript(l, script1)
	err2 := epicexec.RunScript(l, script2)
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
