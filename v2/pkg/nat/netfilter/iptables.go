package netfilter

import (
	"fmt"
	"net"
	"strconv"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"

	"github.com/cybozu-go/coil/v2/pkg/fou"
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

// setIPTablesPeerSrcRule drops packets from the FoU tunnel link of a client
// unless their source address is the client's address.  This prevents
// clients from spoofing the source address of the tunneled packets.
// The rule is set in both FORWARD and INPUT chains to cover packets
// forwarded by and delivered to the egress Pod.
//
// NOTE: The metrics collector takes the counter of the first rule in the
// FORWARD chain as the one for invalid packets.  This rule must be appended
// after that rule.
func setIPTablesPeerSrcRule(family int, linkName string, ip net.IP) error {
	ipt, err := newIPTables(family)
	if err != nil {
		return err
	}

	spec := peerSrcRuleSpec(linkName, ip)
	for _, chain := range []string{filterChain, inputChain} {
		if err := ipt.AppendUnique(filterTable, chain, spec...); err != nil {
			return fmt.Errorf("failed to append %q rule in chain %q - %q: %w", filterTable, chain, spec, err)
		}
	}
	return nil
}

func removeIPTablesPeerSrcRule(family int, linkName string, ip net.IP) error {
	ipt, err := newIPTables(family)
	if err != nil {
		return err
	}

	spec := peerSrcRuleSpec(linkName, ip)
	for _, chain := range []string{filterChain, inputChain} {
		if err := ipt.DeleteIfExists(filterTable, chain, spec...); err != nil {
			return fmt.Errorf("failed to delete %q rule in chain %q - %q: %w", filterTable, chain, spec, err)
		}
	}
	return nil
}

func peerSrcRuleSpec(linkName string, ip net.IP) []string {
	return []string{"-i", linkName, "!", "-s", netlink.NewIPNet(ip).String(), "-j", "DROP"}
}

// setIPTablesFlowBasedDevNewDropRule drops new flows from the flow based
// tunnel device of NAT clients, both forwarded ones and ones destined to
// the client itself.
func setIPTablesFlowBasedDevNewDropRule(family int) error {
	ipt, err := newIPTables(family)
	if err != nil {
		return err
	}

	spec := []string{"-i", flowBasedDevice(family), "-m", "conntrack", "--ctstate", "NEW", "-j", "DROP"}
	for _, chain := range []string{filterChain, inputChain} {
		if err := ipt.AppendUnique(filterTable, chain, spec...); err != nil {
			return fmt.Errorf("failed to append %q rule in chain %q - %q: %w", filterTable, chain, spec, err)
		}
	}
	return nil
}

func flowBasedDevice(family int) string {
	if family == netlink.FAMILY_V6 {
		return fou.FlowBasedIP6TunDevice
	}
	return fou.FlowBasedIP4TunDevice
}

func newIPTables(family int) (*iptables.IPTables, error) {
	ipp, err := netlinkToIptablesFamily(family)
	if err != nil {
		return nil, err
	}
	return iptables.NewWithProtocol(ipp)
}

func setIPTablesConnmarkRules(family int, link netlink.Link) error {
	ipp, err := netlinkToIptablesFamily(family)
	if err != nil {
		return err
	}

	ipt, err := iptables.NewWithProtocol(ipp)
	if err != nil {
		return err
	}

	inputSpec := []string{
		"-i", link.Attrs().Name,
		"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED",
		"-j", "CONNMARK", "--set-mark", strconv.Itoa(link.Attrs().Index),
	}
	if err := ipt.AppendUnique(mangleTable, inputChain, inputSpec...); err != nil {
		return fmt.Errorf("failed to append %q rule in chain %q - %q: %w", mangleTable, inputChain, inputSpec, err)
	}

	outputSpec := []string{"-j", "CONNMARK", "-m", "connmark", "--mark", strconv.Itoa(link.Attrs().Index), "--restore-mark"}
	if err := ipt.AppendUnique(mangleTable, outputChain, outputSpec...); err != nil {
		return fmt.Errorf("failed to append %q rule in chain %q - %q: %w", mangleTable, outputChain, outputSpec, err)
	}
	return nil
}

func removeIPTablesConnmarkRules(family int, link netlink.Link) error {
	ipp, err := netlinkToIptablesFamily(family)
	if err != nil {
		return err
	}
	ipt, err := iptables.NewWithProtocol(ipp)
	if err != nil {
		return err
	}

	inputSpec := []string{
		"-i", link.Attrs().Name,
		"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED",
		"-j", "CONNMARK", "--set-mark", strconv.Itoa(link.Attrs().Index),
	}
	if err := ipt.DeleteIfExists(mangleTable, inputChain, inputSpec...); err != nil {
		return fmt.Errorf("failed to delete %q rule in chain %q - %q: %w", mangleTable, inputChain, inputSpec, err)
	}

	outputSpec := []string{"-j", "CONNMARK", "-m", "connmark", "--mark", strconv.Itoa(link.Attrs().Index), "--restore-mark"}
	if err := ipt.DeleteIfExists(mangleTable, outputChain, outputSpec...); err != nil {
		return fmt.Errorf("failed to delete IPTables rule: %w", err)
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
