package netfilter

import (
	"fmt"
	"net"
	"strconv"

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

func setIPTablesConnmarkRules(family int, link netlink.Link) error {
	ipp, err := netlinkToIptablesFamily(family)
	if err != nil {
		return err
	}

	ipt, err := iptables.NewWithProtocol(ipp)
	if err != nil {
		return err
	}

	inputSpec := []string{"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED", "-j", "CONNMARK",
		"-i", link.Attrs().Name, "--set-mark", strconv.Itoa(link.Attrs().Index)}
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

	inputSpec := []string{"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED", "-j", "CONNMARK",
		"-i", link.Attrs().Name, "--set-mark", strconv.Itoa(link.Attrs().Index)}
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
