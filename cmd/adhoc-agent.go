package cmd

import (
	"net"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"

	"gitlab.com/acnodal/epic/resource-model/controllers"
	"gitlab.com/acnodal/epic/resource-model/internal/exec"
	"gitlab.com/acnodal/epic/resource-model/internal/network"
	"gitlab.com/acnodal/epic/resource-model/internal/pfc"
	// +kubebuilder:scaffold:imports
)

var (
	ipAddress   string
	nicName     string
	accountName string

	// adhocAgentCmd is the adhoc agent subcommand. An instance of the
	// adhoc agent runs on each adhoc linux backend host and sets up the
	// PFC tunnels on that host.
	adhocAgentCmd = &cobra.Command{
		Use:   "adhoc-agent",
		Short: "Run the EPIC adhoc linux node agent",
		RunE:  runAdhocAgent,
	}
)

func init() {
	var (
		defIP  net.IP = net.ParseIP("10.0.0.1")
		defNIC string = "eth0"
	)

	// Find the default outbound IP address.
	if outboundIP, err := network.GetOutboundIP(); err == nil {
		defIP = outboundIP
		// Find which NIC has this IP.
		if outboundNIC, err := network.GetInterfaceByIP(defIP); err == nil {
			defNIC = outboundNIC
		}
	}
	adhocAgentCmd.Flags().StringVarP(&ipAddress, "ip-address", "i", defIP.String(), "Tunnel IP address")
	adhocAgentCmd.Flags().StringVarP(&nicName, "nic", "n", defNIC, "Tunnel network interface name")

	adhocAgentCmd.PersistentFlags().StringVar(&accountName, "account-name", "root", "name of the user account")

	Root.AddCommand(adhocAgentCmd)
}

func runAdhocAgent(cmd *cobra.Command, args []string) error {
	setupLog := ctrl.Log.WithName("setup")
	setupLog.Info("Agent running")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
		LeaderElection:     false,
		Namespace:          "epic-" + accountName,
	})
	if err != nil {
		return err
	}

	// Set up controllers
	if err = (&controllers.GWProxyAdhocReconciler{
		Client:        mgr.GetClient(),
		RuntimeScheme: mgr.GetScheme(),
		NodeAddress:   ipAddress,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	setupLog.V(1).Info("Cleaning up TrueIngress")

	// Empty the PFC tables.
	if err := exec.RunScript(setupLog, "/opt/acnodal/bin/pfc_cli_go initialize"); err != nil {
		return err
	}

	setupLog.V(1).Info("Setting up TrueIngress")

	// Hook TrueIngress into the network interface.
	if err := pfc.SetupNIC(setupLog, nicName, "decap", "ingress", 0, 9); err != nil {
		setupLog.Error(err, "Failed to setup NIC "+nicName)
	}
	if err := pfc.SetupNIC(setupLog, nicName, "encap", "egress", 1, 28); err != nil {
		setupLog.Error(err, "Failed to setup NIC "+nicName)
	}

	setupLog.V(1).Info("starting agent")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return err
	}
	setupLog.V(1).Info("agent returned, will exit")

	return nil
}
