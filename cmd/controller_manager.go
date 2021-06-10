package cmd

import (
	"context"
	"fmt"
	"time"

	marin3r "github.com/3scale/marin3r/apis/marin3r/v1alpha1"
	marin3roperator "github.com/3scale/marin3r/apis/operator/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/controllers"
	"gitlab.com/acnodal/epic/resource-model/internal/allocator"
	// +kubebuilder:scaffold:imports
)

var (
	leaderElect bool
	alloc       *allocator.Allocator
	scheme      = runtime.NewScheme()

	// ControllerManCmd is the controller manager subcommand.
	controllerManCmd = &cobra.Command{
		Use:   "controller-manager",
		Short: "Run the EPIC controller manager",
		RunE:  runControllers,
	}
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch;update
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers,verbs=get;list
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update

func init() {
	utilruntime.Must(marin3r.AddToScheme(scheme))
	utilruntime.Must(marin3roperator.AddToScheme(scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(epicv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme

	// Controllers flags
	controllerManCmd.Flags().BoolVar(&leaderElect, "leader-elect", false,
		"Enabling this will ensure that there is only one active controller manager instance")

	rootCmd.AddCommand(controllerManCmd)
}

func runControllers(cmd *cobra.Command, args []string) error {
	setupLog := ctrl.Log.WithName("setup")
	setupLog.Info("running manager")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     leaderElect,
		LeaderElectionID:   "1cb3972f.acnodal.io",
	})
	if err != nil {
		return err
	}

	// set up address allocator
	alloc = allocator.NewAllocator()

	// Set up controllers and webhooks
	if err = (&controllers.NamespaceReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("Namespace"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err = (&epicv1.EPIC{}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}

	if err = (&epicv1.Account{}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}

	if err = (&controllers.ServicePrefixReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("ServicePrefix"),
		RuntimeScheme: mgr.GetScheme(),
		Allocator:     alloc,
	}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err = (&epicv1.ServicePrefix{}).SetupWebhookWithManager(mgr, alloc); err != nil {
		return err
	}

	if err = (&controllers.LoadBalancerReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("LoadBalancer"),
		RuntimeScheme: mgr.GetScheme(),
		Allocator:     alloc,
	}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err = (&epicv1.LoadBalancer{}).SetupWebhookWithManager(mgr, alloc); err != nil {
		return err
	}

	if err = (&controllers.RemoteEndpointReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("RemoteEndpoint"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}
	if err = (&epicv1.RemoteEndpoint{}).SetupWebhookWithManager(mgr); err != nil {
		return err
	}
	// +kubebuilder:scaffold:builder

	// Clean up data from before we rebooted
	ctx := context.Background()
	if err := prebootCleanup(ctx, setupLog); err != nil {
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return err
	}
	setupLog.Info("manager returned, will exit")
	return nil
}

// prebootCleanup cleans out leftover data that might be
// invalid. Ifindex values, for example, can change after a reboot so
// we zero them and "nudge" the Envoy pods so Python re-writes them.
func prebootCleanup(ctx context.Context, log logr.Logger) error {

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
	list := epicv1.LoadBalancerList{}
	if err = cl.List(ctx, &list); err != nil {
		return err
	}

	// For each LB, wipe out the Status
	for _, lb := range list.Items {
		log.Info("cleanup", "name", lb.Namespace+"/"+lb.Name, "status", lb.Status)

		// Info relating to system network devices is unreliable at this
		// point.
		lb.Status.ProxyInterfaces = map[string]epicv1.ProxyInterfaceInfo{}

		// apply the update
		if err = cl.Status().Update(ctx, &lb); err != nil {
			log.Info("updating LB status", "lb", lb, "error", err)
		}
	}

	// "Nudge" the proxy pods to trigger the python daemon to re-populate the
	// ifindex and veth fields
	proxies, err := listProxyPods(ctx, cl)
	if err != nil {
		return err
	}
	for _, proxyPod := range proxies.Items {
		log.Info("nudge", "name", proxyPod.Namespace+"/"+proxyPod.Name)
		nudgePod(ctx, cl, proxyPod)
	}
	return nil
}

// listProxyPods lists the pods that run our Envoy proxy.
func listProxyPods(ctx context.Context, cl client.Client) (v1.PodList, error) {
	listOps := client.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": epicv1.ProductName, "role": "proxy"}),
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
		return err
	}

	return nil
}
