package cmd

import (
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	"gitlab.com/acnodal/packet-forwarding-component/src/go/pfc"

	"gitlab.com/acnodal/epic/resource-model/controllers"
	"gitlab.com/acnodal/epic/resource-model/internal/exec"
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

	if err = (&controllers.PodAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("PodAgent"),
		RuntimeScheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	if err = (&controllers.RemoteEndpointAgentReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("RemoteEndpointAgent"),
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return err
	}
	setupLog.Info("manager returned, will exit")

	return nil
}
