// Package network contains code to manipulate Linux network configuration.
package network

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"

	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	epicexec "gitlab.com/acnodal/epic/resource-model/internal/exec"
	corev1 "k8s.io/api/core/v1"
)

// AddIpsetEntry adds the provided address and ports to the "epic-in"
// set. The last error to happen will be returned.
func AddIpsetEntry(publicaddr string, ports []corev1.ServicePort) (err error) {
	for _, port := range ports {
		cmd := exec.Command("ipset", "-exist", "add", "epic-in", ipsetAddress(publicaddr, port))
		if cmdErr := cmd.Run(); cmdErr != nil {
			err = cmdErr
		}
	}
	return err
}

// DelIpsetEntry deletes the provided address and ports from the
// "epic-in" set. The last error to happen will be returned.
func DelIpsetEntry(publicaddr string, ports []corev1.ServicePort) (err error) {
	for _, port := range ports {
		cmd := exec.Command("ipset", "-exist", "del", "epic-in", ipsetAddress(publicaddr, port))
		if cmdErr := cmd.Run(); cmdErr != nil {
			err = cmdErr
		}
	}
	return err
}

// ipsetAddress formats an address and port how ipset likes them,
// e.g., "192.168.77.2,tcp:8088". From what I can tell, ipset accepts
// the protocol in upper-case and lower-case so we don't need to do
// anything with it here.
func ipsetAddress(publicaddr string, port corev1.ServicePort) string {
	return fmt.Sprintf("%s,%s:%d", publicaddr, port.Protocol, port.Port)
}

// IpsetSetCheck runs ipset to create the epic-in table.
func IpsetSetCheck() error {
	cmd := exec.Command("/usr/sbin/ipset", "create", "epic-in", "hash:ip,port")
	return cmd.Run()
}

// IPTablesCheck sets up the iptables rules.
func IPTablesCheck(log logr.Logger, brname string) error {
	var (
		script string
	)

	// Check if the rule is already there and add it if not. iptables
	// returns false if the rule doesn't exist.
	script = fmt.Sprintf("/usr/sbin/iptables --check FORWARD -m set --match-set epic-in dst,dst -j ACCEPT || /usr/sbin/iptables --insert FORWARD -m set --match-set epic-in dst,dst -j ACCEPT")
	if err := epicexec.RunScript(log, script); err != nil {
		return err
	}

	script = fmt.Sprintf("/usr/sbin/iptables --check FORWARD -i %s -m comment --comment \"multus bridge %s\" -j ACCEPT || /usr/sbin/iptables --insert FORWARD -i %s -m comment --comment \"multus bridge %s\" -j ACCEPT", brname, brname, brname, brname)
	return epicexec.RunScript(log, script)
}

// AddRt adds a route to `publicaddr` to the interface named
// `multusint`.
func AddRt(publicaddr *net.IPNet, link *netlink.Bridge) error {
	//FIXME error handling, if route add fails should be retried, requeue?
	return netlink.RouteReplace(&netlink.Route{LinkIndex: link.Index, Dst: publicaddr})
}

// DelRt deletes the route to `publicaddr`.
func DelRt(publicaddr *net.IPNet) error {
	//FIXME error handling, if route add fails should be retried, requeue?
	return netlink.RouteDel(&netlink.Route{Dst: publicaddr})
}

// ConfigureBridge configures the bridge interface used to get packets
// from the Envoy pods to the customer endpoints. It's called
// "multus0" by default but that can be overridden in the
// ServicePrefix CR.
func ConfigureBridge(log logr.Logger, brname string, gateway *netlink.Addr) (*netlink.Bridge, error) {
	var err error

	bridge, err := ensureBridge(log, brname, gateway)
	if err != nil {
		log.Error(err, "Multus", "unable to setup", brname)
	}

	if err := IpsetSetCheck(); err == nil {
		log.Info("IPset epic-in added")
	} else {
		log.Info("IPset epic-in already exists")
	}

	if err := IPTablesCheck(log, brname); err == nil {
		log.Info("IPtables setup for multus")
	} else {
		log.Error(err, "iptables", "unable to setup", brname)
	}

	return bridge, err
}

// ensureBridge creates the bridge if it's not there and configures it in either case.
func ensureBridge(log logr.Logger, brname string, gateway *netlink.Addr) (*netlink.Bridge, error) {

	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name:   brname,
			TxQLen: -1,
		},
	}

	// create the bridge if it doesn't already exist
	_, err := netlink.LinkByName(brname)
	if err != nil {
		err = netlink.LinkAdd(br)
		if err != nil && err != syscall.EEXIST {
			log.Info("Multus Bridge could not add %q: %v", brname, err)
		}
	} else {
		log.Info(brname + " already exists")
	}

	// bring the interface up
	if err := netlink.LinkSetUp(br); err != nil {
		return nil, err
	}

	// Proxy ARP is required when we use a device route for the default gateway
	_, err = sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/proxy_arp", brname), "1")
	if err != nil {
		return nil, err
	}

	// add the gateway address to the bridge interface
	if gateway != nil {
		if err := netlink.AddrReplace(br, gateway); err != nil {
			return nil, fmt.Errorf("could not add %v: to %v %w", gateway, br, err)
		}
	}

	return br, nil
}
