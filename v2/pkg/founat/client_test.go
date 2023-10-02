package founat

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

func TestClient(t *testing.T) {
	t.Run("Default", testClientDual)
	t.Run("IPv4", testClientV4)
	t.Run("IPv6", testClientV6)
	t.Run("Custom", testClientCustom)
}

func ruleMap(family int) (map[int]*netlink.Rule, error) {
	rules, err := netlink.RuleList(family)
	if err != nil {
		return nil, err
	}
	m := make(map[int]*netlink.Rule)
	for _, r := range rules {
		r := r
		m[r.Priority] = &r
	}
	return m, nil
}

func testClientDual(t *testing.T) {
	t.Parallel()

	cNS, err := ns.GetNS("/run/netns/test-client-dual")
	if err != nil {
		t.Fatal(err)
	}
	defer cNS.Close()

	err = cNS.Do(func(ns.NetNS) error {
		nc := NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), nil)
		initialized, err := nc.IsInitialized()
		if err != nil {
			return err
		}
		if initialized {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := nc.Init(); err != nil {
			return err
		}

		rm, err := ruleMap(netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if r, ok := rm[1800]; !ok {
			return errors.New("no ipv4 link local rule")
		} else {
			if r.Table != 254 {
				return errors.New("link local rule should point the main table")
			}
		}
		if r, ok := rm[1900]; !ok {
			return errors.New("no ipv4 narrow rule")
		} else {
			if r.Table != 117 {
				return errors.New("narrow rule should point routing table 117")
			}
		}
		if r, ok := rm[2000]; !ok {
			return errors.New("no rule for IPv4 private networks")
		} else {
			if r.Table != 254 {
				return errors.New("private network rule should point the main table")
			}
		}
		if _, ok := rm[2002]; !ok {
			return errors.New("no rule exists for priority 2002")
		}
		if r, ok := rm[2100]; !ok {
			return errors.New("no ipv4 wide rule")
		} else {
			if r.Table != 118 {
				return errors.New("wide rule should point routing table 118")
			}
		}

		rm, err = ruleMap(netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		if r, ok := rm[1800]; !ok {
			return errors.New("no ipv6 link local rule")
		} else {
			if r.Table != 254 {
				return errors.New("link local rule should point the main table")
			}
		}
		if r, ok := rm[1900]; !ok {
			return errors.New("no ipv6 narrow rule")
		} else {
			if r.Table != 117 {
				return errors.New("narrow rule should point routing table 117")
			}
		}
		if r, ok := rm[2000]; !ok {
			return errors.New("no rule for IPv6 private networks")
		} else {
			if r.Table != 254 {
				return errors.New("private network rule should point the main table")
			}
		}
		if r, ok := rm[2100]; !ok {
			return errors.New("no ipv6 wide rule")
		} else {
			if r.Table != 118 {
				return errors.New("wide rule should point routing table 118")
			}
		}

		initialized, err = nc.IsInitialized()
		if err != nil {
			return err
		}
		if !initialized {
			return errors.New("expect to be initialized, but it's not been done")
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		dummy := &netlink.Dummy{LinkAttrs: attrs}
		if err := netlink.LinkAdd(dummy); err != nil {
			return fmt.Errorf("failed to add dummy link: %w", err)
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return fmt.Errorf("failed to get dummy1: %w", err)
		}
		err = nc.AddEgress(link, []*net.IPNet{
			{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
			{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
			{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
		})
		if err != nil {
			return fmt.Errorf("failed to add egress: %w", err)
		}

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv4 dst to table 117")
		}
		if !routes[0].Dst.IP.Equal(net.ParseIP("10.1.2.0")) {
			return fmt.Errorf("wrong dst in table 117: %s", routes[0].Dst.String())
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv4 dst to table 118")
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}

		if len(routes) != 1 {
			return errors.New("failed to add ipv6 dst to table 117")
		}
		if !routes[0].Dst.IP.Equal(net.ParseIP("fd02::")) {
			return fmt.Errorf("wrong dst in table 117: %s", routes[0].Dst.String())
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv6 dst to table 118")
		}

		// Update the destinations
		err = nc.AddEgress(link, []*net.IPNet{
			{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("10.1.3.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("9.9.9.9"), Mask: net.CIDRMask(32, 32)},
			{IP: net.ParseIP("fd03::"), Mask: net.CIDRMask(64, 128)},
			{IP: net.ParseIP("fd04::"), Mask: net.CIDRMask(64, 128)},
			{IP: net.ParseIP("fd05::"), Mask: net.CIDRMask(64, 128)},
			{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
		})

		if err != nil {
			return fmt.Errorf("failed to add egress: %w", err)
		}

		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 2 {
			return errors.New("failed to update ipv4 dst in table 117")
		}
		expectedIPs := make(map[string]struct{})
		for _, route := range routes {
			expectedIPs[route.Dst.IP.String()] = struct{}{}
		}
		if _, ok := expectedIPs["10.1.2.0"]; !ok {
			return fmt.Errorf("wrong dst in table 117: 10.1.2.0 not included")
		}
		if _, ok := expectedIPs["10.1.3.0"]; !ok {
			return fmt.Errorf("wrong dst in table 117: 10.1.3.0 not included")
		}

		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv4 dst to table 118")
		}
		if !routes[0].Dst.IP.Equal(net.ParseIP("9.9.9.9")) {
			return fmt.Errorf("wrong dst in table 118: %s", routes[0].Dst.String())
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}

		if len(routes) != 3 {
			return errors.New("failed to update ipv6 dst in table 117")
		}
		expectedIPs = make(map[string]struct{})
		for _, route := range routes {
			expectedIPs[route.Dst.IP.String()] = struct{}{}
		}
		if _, ok := expectedIPs["fd03::"]; !ok {
			return fmt.Errorf("wrong dst in table 117: fd03:: not included")
		}
		if _, ok := expectedIPs["fd04::"]; !ok {
			return fmt.Errorf("wrong dst in table 117: fd04:: not included")
		}
		if _, ok := expectedIPs["fd05::"]; !ok {
			return fmt.Errorf("wrong dst in table 117: fd05:: not included")
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv6 dst to table 118")
		}
		// NATClient can be re-initialized
		if err := nc.Init(); err != nil {
			return fmt.Errorf("failed to re-initialize NATClient: %w", err)
		}

		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return fmt.Errorf("routing table 117 should be cleared for IPv4: %v", routes)
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return fmt.Errorf("routing table 118 should be cleared for IPv4: %v", routes)
		}

		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return fmt.Errorf("routing table 117 should be cleared for IPv6: %v", routes)
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 118}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return fmt.Errorf("routing table 118 should be cleared for IPv6: %v", routes)
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testClientV4(t *testing.T) {
	t.Parallel()

	cNS, err := ns.GetNS("/run/netns/test-client-v4")
	if err != nil {
		t.Fatal(err)
	}
	defer cNS.Close()

	err = cNS.Do(func(ns.NetNS) error {
		nc := NewNatClient(net.ParseIP("10.1.1.1"), nil, nil)
		initialized, err := nc.IsInitialized()
		if err != nil {
			return err
		}
		if initialized {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := nc.Init(); err != nil {
			return err
		}

		rm, err := ruleMap(netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if _, ok := rm[1800]; !ok {
			return errors.New("no ipv4 link local rule")
		}

		rm, err = ruleMap(netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		if _, ok := rm[1800]; ok {
			return errors.New("ipv6 link local rule exists")
		}

		initialized, err = nc.IsInitialized()
		if err != nil {
			return err
		}
		if !initialized {
			return errors.New("expect to be initialized, but it's not been done")
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		dummy := &netlink.Dummy{LinkAttrs: attrs}
		if err := netlink.LinkAdd(dummy); err != nil {
			return fmt.Errorf("failed to add dummy link: %w", err)
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return fmt.Errorf("failed to get dummy1: %w", err)
		}
		err = nc.AddEgress(link, []*net.IPNet{
			{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
			{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
			{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
		})
		if err != nil {
			return fmt.Errorf("failed to add egress: %w", err)
		}

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv4 dst to table 117")
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return errors.New("ipv6 should be ignored")
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testClientV6(t *testing.T) {
	t.Parallel()

	cNS, err := ns.GetNS("/run/netns/test-client-v6")
	if err != nil {
		t.Fatal(err)
	}
	defer cNS.Close()

	err = cNS.Do(func(ns.NetNS) error {
		nc := NewNatClient(nil, net.ParseIP("fd02::1"), nil)
		initialized, err := nc.IsInitialized()
		if err != nil {
			return err
		}
		if initialized {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := nc.Init(); err != nil {
			return err
		}

		rm, err := ruleMap(netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if _, ok := rm[1800]; ok {
			return errors.New("ipv4 link local rule exists")
		}

		rm, err = ruleMap(netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		if _, ok := rm[1800]; !ok {
			return errors.New("no ipv6 link local rule")
		}

		initialized, err = nc.IsInitialized()
		if err != nil {
			return err
		}
		if !initialized {
			return errors.New("expect to be initialized, but it's not been done")
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		dummy := &netlink.Dummy{LinkAttrs: attrs}
		if err := netlink.LinkAdd(dummy); err != nil {
			return fmt.Errorf("failed to add dummy link: %w", err)
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return fmt.Errorf("failed to get dummy1: %w", err)
		}
		err = nc.AddEgress(link, []*net.IPNet{
			{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
			{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
			{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
		})
		if err != nil {
			return fmt.Errorf("failed to add egress: %w", err)
		}

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return errors.New("ipv4 should be ignored")
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 1 {
			return errors.New("failed to add ipv6 dst to table 117")
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testClientCustom(t *testing.T) {
	t.Parallel()

	cNS, err := ns.GetNS("/run/netns/test-client-custom")
	if err != nil {
		t.Fatal(err)
	}
	defer cNS.Close()

	err = cNS.Do(func(ns.NetNS) error {
		nc := NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), []*net.IPNet{
			{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
		})
		initialized, err := nc.IsInitialized()
		if err != nil {
			return err
		}
		if initialized {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := nc.Init(); err != nil {
			return err
		}

		initialized, err = nc.IsInitialized()
		if err != nil {
			return err
		}
		if !initialized {
			return errors.New("expect to be initialized, but it's not been done")
		}

		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		dummy := &netlink.Dummy{LinkAttrs: attrs}
		if err := netlink.LinkAdd(dummy); err != nil {
			return fmt.Errorf("failed to add dummy link: %w", err)
		}
		link, err := netlink.LinkByName("dummy1")
		if err != nil {
			return fmt.Errorf("failed to get dummy1: %w", err)
		}
		err = nc.AddEgress(link, []*net.IPNet{
			{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
			{IP: net.ParseIP("fd03::"), Mask: net.CIDRMask(64, 128)},
		})
		if err != nil {
			return fmt.Errorf("failed to add egress: %w", err)
		}

		routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return errors.New("should respect custom IPv4 private networks")
		}
		routes, err = netlink.RouteListFiltered(netlink.FAMILY_V6, &netlink.Route{Table: 117}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}
		if len(routes) != 0 {
			return errors.New("should respect custom IPv6 private networks")
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
