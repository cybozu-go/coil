package founat

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
)

func TestEgress(t *testing.T) {
	t.Run("Dual", testEgressDual)
	t.Run("IPv4", testEgressV4)
	t.Run("IPv6", testEgressV6)
}

func testEgressDual(t *testing.T) {
	t.Parallel()

	eNS, err := ns.GetNS("/run/netns/test-egress-dual")
	if err != nil {
		t.Fatal(err)
	}
	defer eNS.Close()

	err = eNS.Do(func(ns.NetNS) error {
		eg := NewEgress("lo", net.ParseIP("127.0.0.1"), net.ParseIP("::1"))
		if err := eg.Init(); err != nil {
			return fmt.Errorf("eg.Init failed: %w", err)
		}

		// test initialization twice
		if err := eg.Init(); err != nil {
			return fmt.Errorf("eg.Init again failed: %w", err)
		}

		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return err
		}
		exist, err := ipt.Exists("nat", "POSTROUTING", "!", "-s", "127.0.0.1/32", "-o", "lo", "-j", "MASQUERADE")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("NAT rule not found for IPv4")
		}
		exist, err = ipt.Exists("filter", "FORWARD", "-o", "lo", "-m", "state", "--state", "INVALID", "-j", "DROP")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("Filter rule not found for IPv4")
		}

		ipt, err = iptables.NewWithProtocol(iptables.ProtocolIPv6)
		if err != nil {
			return err
		}
		exist, err = ipt.Exists("nat", "POSTROUTING", "!", "-s", "::1/128", "-o", "lo", "-j", "MASQUERADE")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("NAT rule not found for IPv6")
		}

		exist, err = ipt.Exists("filter", "FORWARD", "-o", "lo", "-m", "state", "--state", "INVALID", "-j", "DROP")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("Filter rule not found for IPv6")
		}

		rm, err := ruleMap(netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if r, ok := rm[2000]; !ok {
			return errors.New("no ip rule 2000 for IPv4")
		} else {
			if r.Table != 118 {
				return fmt.Errorf("wrong table for IPv4 rule: %d", r.Table)
			}
			if r.IifName != "lo" {
				return fmt.Errorf("wrong incoming interface for IPv4 rule: %s", r.IifName)
			}
		}

		rm, err = ruleMap(netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		if r, ok := rm[2000]; !ok {
			return errors.New("no ip rule 2000 for IPv6")
		} else {
			if r.Table != 118 {
				return fmt.Errorf("wrong table for IPv6 rule: %d", r.Table)
			}
			if r.IifName != "lo" {
				return fmt.Errorf("wrong incoming interface for IPv6 rule: %s", r.IifName)
			}
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
			return err
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return err
		}

		if err := eg.AddClient(net.ParseIP("10.1.2.3"), link); err != nil {
			return fmt.Errorf("failed to call AddClient with 10.1.2.3: %w", err)
		}
		if err := eg.AddClient(net.ParseIP("fd02::1"), link); err != nil {
			return fmt.Errorf("failed to call AddClient with fd02::1: %w", err)
		}

		// call again
		if err := eg.AddClient(net.ParseIP("10.1.2.3"), link); err != nil {
			return fmt.Errorf("failed to call again AddClient with 10.1.2.3: %w", err)
		}
		if err := eg.AddClient(net.ParseIP("fd02::1"), link); err != nil {
			return fmt.Errorf("failed to call again AddClient with fd02::1: %w", err)
		}

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return fmt.Errorf("unexpected routes for IPv4: %v", routes)
		}

		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return fmt.Errorf("unexpected routes for IPv6: %v", routes)
		}

		return nil
	})

	if err != nil {
		t.Error(err)
	}
}

func testEgressV4(t *testing.T) {
	t.Parallel()

	eNS, err := ns.GetNS("/run/netns/test-egress-v4")
	if err != nil {
		t.Fatal(err)
	}
	defer eNS.Close()

	err = eNS.Do(func(ns.NetNS) error {
		eg := NewEgress("lo", net.ParseIP("127.0.0.1"), nil)
		if err := eg.Init(); err != nil {
			return fmt.Errorf("eg.Init failed: %w", err)
		}

		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return err
		}
		exist, err := ipt.Exists("nat", "POSTROUTING", "!", "-s", "127.0.0.1/32", "-o", "lo", "-j", "MASQUERADE")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("NAT rule not found for IPv4")
		}

		exist, err = ipt.Exists("filter", "FORWARD", "-o", "lo", "-m", "state", "--state", "INVALID", "-j", "DROP")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("Filter rule not found for IPv4")
		}

		ipt, err = iptables.NewWithProtocol(iptables.ProtocolIPv6)
		if err != nil {
			return err
		}
		exist, err = ipt.Exists("nat", "POSTROUTING", "!", "-s", "::1/128", "-o", "lo", "-j", "MASQUERADE")
		if err != nil {
			return err
		}
		if exist {
			return errors.New("NAT rule found for IPv6")
		}

		exist, err = ipt.Exists("filter", "FORWARD", "-o", "lo", "-m", "state", "--state", "INVALID", "-j", "DROP")
		if err != nil {
			return err
		}
		if exist {
			return errors.New("Filter rule found for IPv6")
		}

		rm, err := ruleMap(netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if r, ok := rm[2000]; !ok {
			return errors.New("no ip rule 2000 for IPv4")
		} else {
			if r.Table != 118 {
				return fmt.Errorf("wrong table for IPv4 rule: %d", r.Table)
			}
			if r.IifName != "lo" {
				return fmt.Errorf("wrong incoming interface for IPv4 rule: %s", r.IifName)
			}
		}

		rm, err = ruleMap(netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		if _, ok := rm[2000]; ok {
			return errors.New("found ip rule 2000 for IPv6")
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
			return err
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return err
		}

		if err := eg.AddClient(net.ParseIP("10.1.2.3"), link); err != nil {
			return fmt.Errorf("failed to call AddClient with 10.1.2.3: %w", err)
		}
		if err := eg.AddClient(net.ParseIP("fd02::1"), link); err != ErrIPFamilyMismatch {
			return fmt.Errorf("unexpected error: %T", err)
		}

		return nil
	})

	if err != nil {
		t.Error(err)
	}
}

func testEgressV6(t *testing.T) {
	t.Parallel()

	eNS, err := ns.GetNS("/run/netns/test-egress-v6")
	if err != nil {
		t.Fatal(err)
	}
	defer eNS.Close()

	err = eNS.Do(func(ns.NetNS) error {
		eg := NewEgress("lo", nil, net.ParseIP("::1"))
		if err := eg.Init(); err != nil {
			return fmt.Errorf("eg.Init failed: %w", err)
		}

		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return err
		}
		exist, err := ipt.Exists("nat", "POSTROUTING", "!", "-s", "127.0.0.1/32", "-o", "lo", "-j", "MASQUERADE")
		if err != nil {
			return err
		}
		if exist {
			return errors.New("NAT rule found for IPv4")
		}

		exist, err = ipt.Exists("filter", "FORWARD", "-o", "lo", "-m", "state", "--state", "INVALID", "-j", "DROP")
		if err != nil {
			return err
		}
		if exist {
			return errors.New("Filter rule found for IPv4")
		}

		ipt, err = iptables.NewWithProtocol(iptables.ProtocolIPv6)
		if err != nil {
			return err
		}
		exist, err = ipt.Exists("nat", "POSTROUTING", "!", "-s", "::1/128", "-o", "lo", "-j", "MASQUERADE")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("NAT rule not found for IPv6")
		}

		exist, err = ipt.Exists("filter", "FORWARD", "-o", "lo", "-m", "state", "--state", "INVALID", "-j", "DROP")
		if err != nil {
			return err
		}
		if !exist {
			return errors.New("Filter rule not found for IPv6")
		}

		rm, err := ruleMap(netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if _, ok := rm[2000]; ok {
			return errors.New("found ip rule 2000 for IPv4")
		}

		rm, err = ruleMap(netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		if r, ok := rm[2000]; !ok {
			return errors.New("no ip rule 2000 for IPv6")
		} else {
			if r.Table != 118 {
				return fmt.Errorf("wrong table for IPv6 rule: %d", r.Table)
			}
			if r.IifName != "lo" {
				return fmt.Errorf("wrong incoming interface for IPv6 rule: %s", r.IifName)
			}
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
			return err
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return err
		}

		if err := eg.AddClient(net.ParseIP("10.1.2.3"), link); err != ErrIPFamilyMismatch {
			return fmt.Errorf("unexpected error: %T", err)
		}
		if err := eg.AddClient(net.ParseIP("fd02::1"), link); err != nil {
			return fmt.Errorf("failed to call AddClient with fd02::1: %w", err)
		}

		return nil
	})

	if err != nil {
		t.Error(err)
	}
}
