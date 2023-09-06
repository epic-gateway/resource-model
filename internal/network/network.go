// Package network contains code to manipulate Linux network configuration.
package network

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	epicexec "epic-gateway.org/resource-model/internal/exec"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
)

// AddIpsetEntry adds the provided address and ports to the
// appropriate set based on the IP family.
func AddIpsetEntry(l logr.Logger, publicaddr string, ports []corev1.ServicePort) (err error) {
	addr := net.ParseIP(publicaddr)
	if addr == nil {
		return fmt.Errorf("Unparseable IP address: %s", publicaddr)
	}
	for _, port := range ports {
		if nil == addr.To4() {
			epicexec.RunScript(l, "ipset -exist add epic-in-ip6 "+ipsetAddress(publicaddr, port))
		} else {
			epicexec.RunScript(l, "ipset -exist add epic-in     "+ipsetAddress(publicaddr, port))
		}
	}
	return
}

// DelIpsetEntry deletes the provided address and ports from the
// appropriate set based on the IP family.
func DelIpsetEntry(l logr.Logger, publicaddr string, ports []corev1.ServicePort) (err error) {
	addr := net.ParseIP(publicaddr)
	if addr == nil {
		return fmt.Errorf("Unparseable IP address: %s", publicaddr)
	}
	for _, port := range ports {
		if nil == addr.To4() {
			epicexec.RunScript(l, "ipset -exist del epic-in-ip6 "+ipsetAddress(publicaddr, port))
		} else {
			epicexec.RunScript(l, "ipset -exist del epic-in     "+ipsetAddress(publicaddr, port))
		}
	}
	return
}

// ipsetAddress formats an address and port how ipset likes them,
// e.g., "192.168.77.2,tcp:8088". From what I can tell, ipset accepts
// the protocol in upper-case and lower-case so we don't need to do
// anything with it here.
func ipsetAddress(publicaddr string, port corev1.ServicePort) string {
	return fmt.Sprintf("%s,%s:%d", publicaddr, port.Protocol, port.Port)
}

// ipsetSetCheck runs ipset to create the epic-in tables.
func ipsetSetCheck(l logr.Logger) error {
	// If the set exists we'll flush it, and if it doesn't exist we'll
	// create it
	err6 := epicexec.RunScript(l, "/usr/sbin/ipset flush epic-in-ip6 || /usr/sbin/ipset create epic-in-ip6 hash:ip,port family inet6")
	err4 := epicexec.RunScript(l, "/usr/sbin/ipset flush epic-in     || /usr/sbin/ipset create epic-in     hash:ip,port family inet ")
	if err6 != nil {
		return err6
	}
	return err4
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

// ConfigureBridge configures the bridge interface used to get packets
// from the Envoy pods to the customer endpoints. It's called
// "multus0" by default but that can be overridden in the
// ServicePrefix CR.
func ConfigureBridge(log logr.Logger, brname string) (*netlink.Bridge, error) {
	var err error

	bridge, err := ensureBridge(log, brname)
	if err != nil {
		log.Error(err, "Multus", "unable to setup", brname)
	}

	if err := ipsetSetCheck(log); err != nil {
		log.Error(err, "IPSet unable to setup")
	}

	if err := IPTablesCheck(log, brname); err != nil {
		log.Error(err, "iptables", "unable to setup", brname)
	}

	return bridge, err
}

// ensureBridge creates the bridge if it's not there and configures it in either case.
func ensureBridge(log logr.Logger, brname string) (*netlink.Bridge, error) {

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

	return br, nil
}

// Get preferred outbound ip of this machine.
// https://stackoverflow.com/a/37382208/5967960
func GetOutboundIP() (net.IP, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).IP, nil
}

// https://stackoverflow.com/a/63205544/5967960
func GetInterfaceByIP(ip net.IP) (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				if iip, _, err := net.ParseCIDR(addr.String()); err == nil {
					if iip.Equal(ip) {
						return iface.Name, nil
					}
				} else {
					continue
				}
			}
		} else {
			continue
		}
	}
	return "", errors.New("couldn't find a interface for the ip")
}
