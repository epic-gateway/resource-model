package controllers

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/internal/pfc"
)

// PodAgentReconciler reconciles a Pod object
type PodAgentReconciler struct {
	client.Client
	RuntimeScheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers,verbs=get;list
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=lbservicegroups,verbs=get;list
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=serviceprefixes,verbs=get;list

// Reconcile takes a Request and makes the system reflect what the
// Request is asking for.
func (r *PodAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx).WithValues("agent-running-on", os.Getenv("EPIC_NODE_NAME"))
	pod := v1.Pod{}
	sg := epicv1.LBServiceGroup{}
	prefix := epicv1.ServicePrefix{}
	result := ctrl.Result{}

	// read the Pod that caused the event
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		l.Info("can't get pod resource, probably deleted")
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return done, client.IgnoreNotFound(err)
	}

	// If it's not an Envoy Pod then do nothing
	if !epicv1.HasEnvoyLabels(pod) {
		l.Info("pod is not a proxy pod")
		return done, nil
	}

	// If it's not running on this node then do nothing
	if os.Getenv("EPIC_NODE_NAME") != pod.Spec.NodeName {
		l.Info("pod is not running on this node", "pod-running-on", pod.Spec.NodeName)
		return done, nil
	}

	// We need this pod's veth info so if the python daemon hasn't
	// filled it in yet then we'll back off and give it more time
	ifIndex, haveIfIndex := pod.ObjectMeta.Annotations[epicv1.IfIndexAnnotation]
	ifName, haveIfName := pod.ObjectMeta.Annotations[epicv1.IfNameAnnotation]
	if !haveIfIndex || !haveIfName {
		l.Info("incomplete proxy interface info, will retry")
		return tryAgain, nil
	}

	l.V(1).Info("Reconciling", "ifindex", ifIndex, "ifname", ifName)
	var (
		publicIP, altIP net.IP
		sgName          types.NamespacedName
	)

	owningProxy, belongsToProxy := pod.Labels[epicv1.OwningProxyLabel]
	if belongsToProxy {
		// Get the Proxy to which this rep belongs
		proxy := epicv1.GWProxy{}
		proxyName := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: owningProxy}
		if err := r.Get(ctx, proxyName, &proxy); err != nil {
			return done, client.IgnoreNotFound(err)
		}

		// Calculate this proxy's public and alt addresses which we use
		// both when we add and when we delete
		publicIP = net.ParseIP(proxy.Spec.PublicAddress)
		if publicIP == nil {
			return done, fmt.Errorf("%s can't be parsed as an IP address", proxy.Spec.PublicAddress)
		}
		altIP = net.ParseIP(proxy.Spec.AltAddress)

		// Find the owning ServiceGroup's name.
		sgName = types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: proxy.Labels[epicv1.OwningLBServiceGroupLabel]}
	}

	owningLB, belongsToLB := pod.Labels[epicv1.OwningLoadBalancerLabel]
	if belongsToLB {
		// Get the LB to which this rep belongs
		lb := epicv1.LoadBalancer{}
		lbname := types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: owningLB}
		if err := r.Get(ctx, lbname, &lb); err != nil {
			return done, err
		}

		// Calculate this LB's public address which we use both when we add
		// and when we delete
		publicIP = net.ParseIP(lb.Spec.PublicAddress)
		if publicIP == nil {
			return done, fmt.Errorf("%s can't be parsed as an IP address", lb.Spec.PublicAddress)
		}

		// Find the owning ServiceGroup's name.
		sgName = types.NamespacedName{Namespace: req.NamespacedName.Namespace, Name: lb.Labels[epicv1.OwningLBServiceGroupLabel]}
	}

	// Get the "stack" of CRs to which this LB/Proxy belongs: LBServiceGroup,
	// and ServicePrefix. They provide configuration data that we need
	// but that isn't contained in the pod or LB/Proxy.
	if err := r.Get(ctx, sgName, &sg); err != nil {
		l.Error(err, "Failed to find owning service group", "sgName", sgName)
		return done, err
	}

	prefixName := types.NamespacedName{Namespace: epicv1.ConfigNamespace, Name: sg.Labels[epicv1.OwningServicePrefixLabel]}
	if err := r.Get(ctx, prefixName, &prefix); err != nil {
		l.Error(err, "Failed to find owning service prefix", "prefixName", prefixName)
		return done, err
	}

	// If this object is *not* being deleted then set up the TrueIngress
	// tagger.
	if pod.ObjectMeta.DeletionTimestamp.IsZero() {
		// add the packet tagger to the Envoy pod veth
		if err := r.configureTagging(l, ifName); err != nil {
			return result, err
		}

		// FIXME: we would like to *not* attract LB traffic to this node
		// until we're certain that it hosts a ready Envoy proxy
		// pod. We're currently prevented from this because the Multus
		// route serves two purposes: attracting LB traffic through the
		// "front door", and attracting PFC traffic returning from the
		// upstream cluster endpoints. We can't enable one without
		// enabling the other, at least until we work out a more precise
		// control scheme for the local router. So for now we'll add the
		// route now so the PFC tunnels work, even though we might lose
		// some traffic between the time that the route is announced and
		// the time that the PFC tunnel becomes fully ready.

		// Attract LB traffic to this node
		if altIP != nil {
			l.Info("Adding route", "proxy-alt-address", altIP.String())
			if err := prefix.Spec.AltPool.AddMultusRoute(altIP); err != nil {
				return done, err
			}
		}
		l.Info("Adding route", "proxy-public-address", publicIP.String())
		if err := prefix.Spec.PublicPool.AddMultusRoute(publicIP); err != nil {
			l.Error(err, "Adding public address multus route", "publicIP", publicIP)
			return done, err
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
