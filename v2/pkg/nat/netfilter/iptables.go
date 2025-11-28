package netfilter

import (
	"fmt"
	"net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
)

func setIPTablesMasqRules(ipp iptables.Protocol, iface string, ip net.IP) error {
	ipn := netlink.NewIPNet(ip)
	ipt, err := iptables.NewWithProtocol(ipp)
	if err != nil {
		return err
	}

	spec := []string{"!", "-s", ipn.String(), "-o", iface, "-j", "MASQUERADE"}
	if err := ipt.Append(natTable, natChain, spec...); err != nil {
		return fmt.Errorf("failed to setup masquerade rule: %w", err)
	}

	// drop invalid packets
	spec = []string{"-o", iface, "-m", "state", "--state", "INVALID", "-j", "DROP"}
	if err := ipt.Append(filterTable, filterChain, spec...); err != nil {
		return fmt.Errorf("failed to setup drop rule for invalid packets: %w", err)
	}
	return nil
}
