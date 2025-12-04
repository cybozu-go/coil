package netfilter

import (
	"fmt"
	"net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
)

func setIPTablesMasqRules(family int, iface string, ip net.IP) error {
	ipn := netlink.NewIPNet(ip)
	ipp, err := netlinkToIptablesFamily(family)
	if err != nil {
		return err
	}
	ipt, err := iptables.NewWithProtocol(ipp)
	if err != nil {
		return err
	}

	spec := []string{"!", "-s", ipn.String(), "-o", iface, "-j", "MASQUERADE"}
	if err := ipt.AppendUnique(natTable, natChain, spec...); err != nil {
		return fmt.Errorf("failed to setup masquerade rule: %w", err)
	}

	// drop invalid packets
	spec = []string{"-o", iface, "-m", "state", "--state", "INVALID", "-j", "DROP"}
	if err := ipt.AppendUnique(filterTable, filterChain, spec...); err != nil {
		return fmt.Errorf("failed to setup drop rule for invalid packets: %w", err)
	}
	return nil
}

func netlinkToIptablesFamily(family int) (iptables.Protocol, error) {
	switch family {
	case netlink.FAMILY_V4:
		return iptables.ProtocolIPv4, nil
	case netlink.FAMILY_V6:
		return iptables.ProtocolIPv6, nil
	default:
		return 0, fmt.Errorf("invalid IP family %d", family)
	}
}
