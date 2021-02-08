package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	marin3roperator "github.com/3scale/marin3r/apis/operator/v1alpha1"
	"gitlab.com/acnodal/packet-forwarding-component/src/go/pfc"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	"gitlab.com/acnodal/egw-resource-model/controllers"
	"gitlab.com/acnodal/egw-resource-model/internal/exec"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch;update
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers,verbs=get;list
// +kubebuilder:rbac:groups=egw.acnodal.io,resources=loadbalancers/status,verbs=get;update

func init() {
	utilruntime.Must(marin3r.AddToScheme(scheme))
	utilruntime.Must(marin3roperator.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(egwv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8081", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "1cb3972f.acnodal.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Set up controllers and webhooks
	if err = (&controllers.NamespaceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Namespace"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Namespace")
		os.Exit(1)
	}

	if err = (&controllers.EGWReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("EGW"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EGW")
		os.Exit(1)
	}
	if err = (&egwv1.EGW{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "EGW")
		os.Exit(1)
	}

	if err = (&controllers.ServicePrefixReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ServicePrefix"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServicePrefix")
		os.Exit(1)
	}

	if err = (&controllers.ServiceGroupReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("ServiceGroup"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceGroup")
		os.Exit(1)
	}

	if err = (&controllers.AccountReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Account"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Account")
		os.Exit(1)
	}
	if err = (&egwv1.Account{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Account")
		os.Exit(1)
	}

	if err = (&controllers.LoadBalancerReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("LoadBalancer"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LoadBalancer")
		os.Exit(1)
	}
	if err = (&egwv1.LoadBalancer{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "LoadBalancer")
		os.Exit(1)
	}

	if err = (&controllers.RemoteEndpointReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("RemoteEndpoint"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RemoteEndpoint")
		os.Exit(1)
	}
	if err = (&egwv1.RemoteEndpoint{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RemoteEndpoint")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	// See if the PFC is installed
	ok, message := pfc.Check()
	if ok {
		// print the version
		setupLog.Info("PFC", "version", message)
	} else {
		setupLog.Info("PFC Error", "message", message)
	}

	// Clean up data from before we rebooted
	ctx := context.Background()
	if err := prebootCleanup(ctx); err != nil {
		setupLog.Error(err, "problem running cleanup")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// prebootCleanup cleans out leftover data that might be
// invalid. Ifindex values, for example, can change after a reboot so
// we want to zero them and wait until Python re-writes them. PFC
// tables should be initialized so we don't send packets to the wrong
// place during startup.
func prebootCleanup(ctx context.Context) error {

	// Empty the PFC tables
	err := exec.RunScript(setupLog, "/opt/acnodal/bin/pfc_cli_go initialize")
	if err != nil {
		return err
	}

	// We use an ad-hoc client here because the mgr.GetClient() doesn't
	// start until you call mgr.Start() but we want to do this cleanup
	// before the controllers start
	config, err := rest.InClusterConfig()
	if err != nil {
		return err
	}
	cl, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return err
	}

	// List all of the LBs
	list := egwv1.LoadBalancerList{}
	if err = cl.List(ctx, &list); err != nil {
		return err
	}

	// For each LB, wipe out the Status
	for _, lb := range list.Items {
		setupLog.Info("cleanup", "name", lb.Namespace+"/"+lb.Name, "status", lb.Status)

		// Info relating to system network devices is unreliable at this
		// point.
		lb.Status.ProxyIfindex = 0
		lb.Status.ProxyIfname = ""

		// apply the update
		if err = cl.Status().Update(ctx, &lb); err != nil {
			setupLog.Info("updating LB status", "lb", lb, "error", err)
		}
	}

	// "Nudge" the proxy pods to trigger the python daemon to re-populate the
	// ifindex and veth fields
	proxies, err := listProxyPods(ctx, cl)
	if err != nil {
		return err
	}
	for _, proxyPod := range proxies.Items {
		setupLog.Info("nudge", "name", proxyPod.Namespace+"/"+proxyPod.Name)
		nudgePod(ctx, cl, proxyPod)
	}
	return nil
}

// listProxyPods lists the pods that run our Envoy proxy.
func listProxyPods(ctx context.Context, cl client.Client) (v1.PodList, error) {
	listOps := client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": "egw", "role": "proxy"}),
	}
	list := v1.PodList{}
	err := cl.List(ctx, &list, &listOps)

	return list, err
}

// nudgePod "nudges" a pod, i.e., triggers an update event by
// adding/modifying an annotation. This causes the setup-network
// daemon to re-scan for the pod's veth and ifindex.
func nudgePod(ctx context.Context, cl client.Client, pod v1.Pod) error {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations["nudge"] = fmt.Sprintf("%x", time.Now().UnixNano())

	// apply the update
	if err := cl.Update(ctx, &pod); err != nil {
		setupLog.Info("nudging pod", "pod", pod, "error", err)
		return err
	}

	return nil
}
