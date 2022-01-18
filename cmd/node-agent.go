package cmd

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	epicv1 "gitlab.com/acnodal/epic/resource-model/api/v1"
	"gitlab.com/acnodal/epic/resource-model/controllers"
	"gitlab.com/acnodal/epic/resource-model/internal/exec"
	"gitlab.com/acnodal/packet-forwarding-component/src/go/pfc"
	// +kubebuilder:scaffold:imports
)

var (
	// nodeAgentCmd is the node agent subcommand. An instance of the
	// node agent runs on each node in the cluster and sets up the PFC
	// tunnels for endpoints on that node.
	nodeAgentCmd = &cobra.Command{
		Use:   "node-agent",
		Short: "Run the EPIC node agent",
		RunE:  runNodeAgent,
	}
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=list;get;watch;update
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers,verbs=get;list
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=loadbalancers/status,verbs=get;update
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies,verbs=get;list
// +kubebuilder:rbac:groups=epic.acnodal.io,resources=gwproxies/status,verbs=get;update

func init() {
	rootCmd.AddCommand(nodeAgentCmd)
}

func runNodeAgent(cmd *cobra.Command, args []string) error {
	setupLog := ctrl.Log.WithName("setup")
	setupLog.Info("running agent")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     false,
		LeaderElectionID:   "1cb3972f.acnodal.io",
	})
	if err != nil {
		return err
	}

	// Set up controllers
	if err = (&controllers.EPICAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("EPICAgent"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err = (&controllers.ServicePrefixAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("ServicePrefixAgent"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err = (&controllers.LoadBalancerAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("LoadBalancerAgent"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err = (&controllers.GWProxyAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("GWProxyAgent"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err = (&controllers.PodAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PodAgent"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	// See if the PFC is installed
	ok, message := pfc.Check()
	if ok {
		// print the version
		setupLog.Info("PFC", "version", message)
	} else {
		setupLog.Info("PFC Error", "message", message)
	}

	// Empty the PFC tables
	if err := exec.RunScript(setupLog, "/opt/acnodal/bin/pfc_cli_go initialize"); err != nil {
		return err
	}

	// Clean up data from before we rebooted
	ctx := context.Background()
	if err := prebootNodeCleanup(ctx, setupLog); err != nil {
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return err
	}
	setupLog.Info("manager returned, will exit")

	return nil
}

// prebootNodeCleanup cleans out leftover data (relevant to this node)
// that might be invalid. Ifindex values, for example, can change
// after a reboot so we zero them and "nudge" the Envoy pods so Python
// re-writes them. See also prebootCleanup() in controller_manager.go
// for the system-wide preboot cleanup.
func prebootNodeCleanup(ctx context.Context, log logr.Logger) error {

	// We use an ad-hoc client here because the mgr.GetClient() doesn't
	// start until you call mgr.Start() but we want to do this cleanup
	// before the controllers start
	cl, err := adHocClient(scheme)
	if err != nil {
		return err
	}

	// "Nudge" the proxy pods to trigger the python daemon to
	// re-populate the ifindex and ifname annotations
	proxies, err := listProxyPods(ctx, cl)
	if err != nil {
		return err
	}
	for _, proxyPod := range proxies.Items {
		// If it's not running on this node then do nothing
		if os.Getenv("EPIC_NODE_NAME") != proxyPod.Spec.NodeName {
			continue
		}

		log.Info("clean", "name", proxyPod.Namespace+"/"+proxyPod.Name)
		cleanIntfAnnotations(ctx, cl, proxyPod.Namespace, proxyPod.Name)
		// Remove the pod's info from the LB but continue even if there's
		// an error.
		epicv1.RemovePodInfo(ctx, cl, proxyPod.Namespace, proxyPod.Labels[epicv1.OwningLoadBalancerLabel], proxyPod.Name)
	}
	return nil
}
