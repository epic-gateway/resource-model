package main

import (
	"flag"
	"fmt" //added for multus0 setup
	"os"
	"os/exec" //added for ipset & iptables
	"syscall" //added for multus0 setup

	"github.com/vishvananda/netlink" //added for multus0 setup

	"github.com/containernetworking/plugins/pkg/utils/sysctl" //added for multus0

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	egwv1 "gitlab.com/acnodal/egw-resource-model/api/v1"
	"gitlab.com/acnodal/egw-resource-model/controllers"
	pfc "gitlab.com/acnodal/packet-forwarding-component/src/go/pfc"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	brname   = "multus0" //should come from netattach defs
)

func init() {
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

	if err = (&controllers.AccountReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Account"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Account")
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
	if err = (&controllers.LoadBalancerReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("LoadBalancer"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LoadBalancer")
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
	if err = (&egwv1.LoadBalancer{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "LoadBalancer")
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

	ok, message = brCheck(brname)
	if ok {
		setupLog.Info("Multus present")
	} else {
		_, err := ensureBridge(brname)
		if err != nil {
			setupLog.Error(err, "Multus", "unable to setup", brname)
		}
	}

	ok, message = ipTablesCheck(brname)
	if ok {
		setupLog.Info("IPtables setup for multus")
	} else {
		setupLog.Error(err, "iptables", "unable to setup", brname)
	}

	ok, message = ipsetSetCheck()
	if ok {
		setupLog.Info("IPset egw-in added")
	} else {
		setupLog.Info("IPset egw-in already exists")
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func ensureBridge(brname string) (*netlink.Bridge, error) {

	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   brname,
			TxQLen: -1,
		},
	}

	err := netlink.LinkAdd(br)
	if err != nil && err != syscall.EEXIST {
		setupLog.Info("Multus Bridge could not add %q: %v", brname, err)
	}

	// This interface will not have an address so we need proxy arp enabled

	_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/proxy_arp", brname), "1")

	if err := netlink.LinkSetUp(br); err != nil {
		return nil, err
	}

	return br, nil
}

func brCheck(brname string) (bool, string) {

	_, err := netlink.LinkByName(brname)
	if err != nil {
		return false, "Multus " + brname + " not found"
	}

	return true, ""
}

func ipsetSetCheck() (bool, string) {
	cmd := exec.Command("ipset", "create", "egw-in", "hash:ip,port")
	err := cmd.Run()
	if err != nil {
		return false, "IPSet failed"
	}

	return true, ""
}

func ipTablesCheck(brname string) (bool, string) {
	cmd := exec.Command("iptables", "-C  FORWARD -i", brname, "-m comment --comment \"multus bridge", brname, "\" -j ACCEPT")
	err := cmd.Run()
	if err != nil {
		cmd := exec.Command("iptables", "-A  FORWARD -i", brname, "-m comment --comment \"multus bridge", brname, "\" -j ACCEPT")
		err := cmd.Run()
		if err != nil {
			return false, "unable to update iptables"
		}
		cmd = exec.Command("iptables", "-A FORWARD -m set --match-set egw-in dst,dst -j ACCEPT")
		err = cmd.Run()
		if err != nil {
			return false, "unable to update iptables"
		}
	}

	return true, ""

}
