package fou

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/vishvananda/netlink"
)

func TestFoU(t *testing.T) {
	t.Run("Dual", testFoUDual)
	t.Run("IPv4", testFoUV4)
	t.Run("IPv6", testFoUV6)
	t.Run("Server", testFoUServer)
}

func testFoUDual(t *testing.T) {
	t.Parallel()

	fNS, err := ns.GetNS("/run/netns/test-fou-dual")
	if err != nil {
		t.Fatal(err)
	}
	defer fNS.Close()

	err = fNS.Do(func(ns.NetNS) error {
		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
			return fmt.Errorf("failed to add dummy1: %w", err)
		}
		dummy, err := netlink.LinkByName("dummy1")
		if err != nil {
			return fmt.Errorf("failed to get dummy1: %w", err)
		}
		err = netlink.AddrAdd(dummy, &netlink.Addr{
			IPNet: &net.IPNet{IP: net.ParseIP("10.1.1.0"), Mask: net.CIDRMask(24, 32)},
		})
		if err != nil {
			return fmt.Errorf("netlink: failed to add an IPv4 address: %w", err)
		}
		err = netlink.AddrAdd(dummy, &netlink.Addr{
			IPNet: &net.IPNet{IP: net.ParseIP("fd02::100"), Mask: net.CIDRMask(120, 128)},
		})
		if err != nil {
			return fmt.Errorf("netlink: failed to add an IPv6 address: %w", err)
		}

		// IPv4 forwarding of a new netns is inherited from the host.
		// Reset it to check that Init does not enable forwarding.
		for _, key := range []string{"net.ipv4.ip_forward", "net.ipv6.conf.all.forwarding"} {
			if _, err := sysctl.Sysctl(key, "0"); err != nil {
				return fmt.Errorf("failed to set %s: %w", key, err)
			}
		}

		fou := NewClientFoUTunnel(5555, net.ParseIP("127.0.0.1"), net.ParseIP("::1"), nil)
		if fou.IsInitialized() {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := fou.Init(); err != nil {
			return fmt.Errorf("fou.Init failed: %w", err)
		}

		if !fou.IsInitialized() {
			return errors.New("expect to be initialized, but it's not been done")
		}

		// test initialization twice
		if err := fou.Init(); err != nil {
			return fmt.Errorf("fou.Init again failed: %w", err)
		}

		// NAT clients do not forward packets
		if err := expectSysctl("net.ipv4.ip_forward", "0"); err != nil {
			return err
		}
		if err := expectSysctl("net.ipv6.conf.all.forwarding", "0"); err != nil {
			return err
		}

		fous, err := netlink.FouList(0)
		if err != nil {
			return fmt.Errorf("failed to list fou links: %w", err)
		}
		if len(fous) != 2 {
			return fmt.Errorf("unexpected fou list: %+v", fous)
		}
		for i, f := range fous {
			if f.Port != 5555 {
				return fmt.Errorf("unexpected fous[%d] port number: %d", i, f.Port)
			}
		}

		if link, err := fou.AddPeer(net.ParseIP("10.1.1.1"), false); err != nil {
			return fmt.Errorf("failed to call AddPeer with 10.1.1.1: %w", err)
		} else {
			iptun, ok := link.(*netlink.Iptun)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !iptun.Remote.Equal(net.ParseIP("10.1.1.1")) {
				return fmt.Errorf("remote is not 10.1.1.1: %s", iptun.Remote.String())
			}
			if !iptun.Local.Equal(net.ParseIP("127.0.0.1")) {
				return fmt.Errorf("local is not 127.0.0.1: %s", iptun.Local.String())
			}
			if iptun.EncapDport != 5555 {
				return fmt.Errorf("iptun.EncapDport is not 5555: %d", iptun.EncapDport)
			}
			if iptun.EncapSport != 5555 {
				return fmt.Errorf("iptun.EncapSport is not 5555: %d", iptun.EncapSport)
			}

			ipip4, err := netlink.LinkByName("coil_ipip4")
			if err != nil {
				return fmt.Errorf("failed to get coil_ipip4: %w", err)
			}
			iptun, ok = ipip4.(*netlink.Iptun)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !iptun.FlowBased {
				return errors.New("coil_ipip4 is not flow based")
			}
		}

		// Update the encap sport setting
		if link, err := fou.AddPeer(net.ParseIP("10.1.1.1"), true); err != nil {
			return fmt.Errorf("failed to call AddPeer with 10.1.1.1: %w", err)
		} else {
			iptun, ok := link.(*netlink.Iptun)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !iptun.Remote.Equal(net.ParseIP("10.1.1.1")) {
				return fmt.Errorf("remote is not 10.1.1.1: %s", iptun.Remote.String())
			}
			if !iptun.Local.Equal(net.ParseIP("127.0.0.1")) {
				return fmt.Errorf("local is not 127.0.0.1: %s", iptun.Local.String())
			}
			if iptun.EncapDport != 5555 {
				return fmt.Errorf("iptun.EncapDport is not 5555: %d", iptun.EncapDport)
			}
			if iptun.EncapSport != 0 {
				return fmt.Errorf("iptun.EncapSport is not 0: %d", iptun.EncapSport)
			}

			ipip4, err := netlink.LinkByName("coil_ipip4")
			if err != nil {
				return fmt.Errorf("failed to get coil_ipip4: %w", err)
			}
			iptun, ok = ipip4.(*netlink.Iptun)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !iptun.FlowBased {
				return errors.New("coil_ipip4 is not flow based")
			}
		}

		if link, err := fou.AddPeer(net.ParseIP("fd02::101"), false); err != nil {
			return fmt.Errorf("failed to call AddPeer with fd02::101: %w", err)
		} else {
			ip6tnl, ok := link.(*netlink.Ip6tnl)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !ip6tnl.Remote.Equal(net.ParseIP("fd02::101")) {
				return fmt.Errorf("remote is not fd02::101: %s", ip6tnl.Remote.String())
			}
			if !ip6tnl.Local.Equal(net.ParseIP("::1")) {
				return fmt.Errorf("local is not ::1: %s", ip6tnl.Local.String())
			}
			if ip6tnl.EncapDport != 5555 {
				return fmt.Errorf("ip6tnl.EncapDport is not 5555: %d", ip6tnl.EncapDport)
			}
			if ip6tnl.EncapSport != 5555 {
				return fmt.Errorf("ip6tnl.EncapSport is not 5555: %d", ip6tnl.EncapSport)
			}

			ipip6, err := netlink.LinkByName("coil_ipip6")
			if err != nil {
				return fmt.Errorf("failed to get coil_ipip6: %w", err)
			}
			ip6tnl, ok = ipip6.(*netlink.Ip6tnl)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !ip6tnl.FlowBased {
				return errors.New("coil_ipip6 is not flow based")
			}
		}

		// Update the encap sport setting
		if link, err := fou.AddPeer(net.ParseIP("fd02::101"), true); err != nil {
			return fmt.Errorf("failed to call AddPeer with fd02::101: %w", err)
		} else {
			ip6tnl, ok := link.(*netlink.Ip6tnl)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !ip6tnl.Remote.Equal(net.ParseIP("fd02::101")) {
				return fmt.Errorf("remote is not fd02::101: %s", ip6tnl.Remote.String())
			}
			if !ip6tnl.Local.Equal(net.ParseIP("::1")) {
				return fmt.Errorf("local is not ::1: %s", ip6tnl.Local.String())
			}
			if ip6tnl.EncapDport != 5555 {
				return fmt.Errorf("ip6tnl.EncapDport is not 5555: %d", ip6tnl.EncapDport)
			}
			if ip6tnl.EncapSport != 0 {
				return fmt.Errorf("ip6tnl.EncapSport is not 0: %d", ip6tnl.EncapSport)
			}

			ipip6, err := netlink.LinkByName("coil_ipip6")
			if err != nil {
				return fmt.Errorf("failed to get coil_ipip6: %w", err)
			}
			ip6tnl, ok = ipip6.(*netlink.Ip6tnl)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !ip6tnl.FlowBased {
				return errors.New("coil_ipip6 is not flow based")
			}
		}
		if err := fou.DelPeer(net.ParseIP("10.1.1.1")); err != nil {
			return fmt.Errorf("failed to call DelPeer with 10.1.1.1: %w", err)
		}
		links, err := netlink.LinkList()
		if err != nil {
			return err
		}
		for _, l := range links {
			if strings.HasPrefix(l.Attrs().Name, FoU4LinkPrefix) {
				return fmt.Errorf("undeleted fou link: %s", l.Attrs().Name)
			}
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testFoUV4(t *testing.T) {
	t.Parallel()

	fNS, err := ns.GetNS("/run/netns/test-fou-v4")
	if err != nil {
		t.Fatal(err)
	}
	defer fNS.Close()

	err = fNS.Do(func(ns.NetNS) error {
		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
			return err
		}
		dummy, err := netlink.LinkByName("dummy1")
		if err != nil {
			return err
		}
		err = netlink.AddrAdd(dummy, &netlink.Addr{
			IPNet: &net.IPNet{IP: net.ParseIP("10.1.1.0"), Mask: net.CIDRMask(24, 32)},
		})
		if err != nil {
			return err
		}

		fou := NewClientFoUTunnel(5555, net.ParseIP("127.0.0.1"), nil, nil)
		if fou.IsInitialized() {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := fou.Init(); err != nil {
			return fmt.Errorf("fou.Init failed: %w", err)
		}

		if !fou.IsInitialized() {
			return errors.New("expect to be initialized, but it's not been done")
		}

		fous, err := netlink.FouList(0)
		if err != nil {
			return fmt.Errorf("failed to list fou links: %w", err)
		}
		if len(fous) != 1 {
			return fmt.Errorf("unexpected fou list: %+v", fous)
		}
		for i, f := range fous {
			if f.Port != 5555 {
				return fmt.Errorf("unexpected fous[%d] port number: %d", i, f.Port)
			}
		}

		if link, err := fou.AddPeer(net.ParseIP("10.1.1.1"), true); err != nil {
			return fmt.Errorf("failed to call AddPeer with 10.1.1.1: %w", err)
		} else {
			iptun, ok := link.(*netlink.Iptun)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !iptun.Remote.Equal(net.ParseIP("10.1.1.1")) {
				return fmt.Errorf("remote is not 10.1.1.1: %s", iptun.Remote.String())
			}
			if !iptun.Local.Equal(net.ParseIP("127.0.0.1")) {
				return fmt.Errorf("local is not 127.0.0.1: %s", iptun.Local.String())
			}
			if iptun.EncapDport != 5555 {
				return fmt.Errorf("iptun.EncapDport is not 5555: %d", iptun.EncapDport)
			}
			if iptun.EncapSport != 0 {
				return fmt.Errorf("iptun.EncapSport is not 0: %d", iptun.EncapSport)
			}

			ipip4, err := netlink.LinkByName("coil_ipip4")
			if err != nil {
				return fmt.Errorf("failed to get coil_ipip4: %w", err)
			}
			iptun, ok = ipip4.(*netlink.Iptun)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !iptun.FlowBased {
				return errors.New("coil_ipip4 is not flow based")
			}
		}

		if _, err := fou.AddPeer(net.ParseIP("fd02::101"), true); err != ErrIPFamilyMismatch {
			return fmt.Errorf("error is not ErrIPFamilyMismatch: %w", err)
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testFoUV6(t *testing.T) {
	t.Parallel()

	fNS, err := ns.GetNS("/run/netns/test-fou-v6")
	if err != nil {
		t.Fatal(err)
	}
	defer fNS.Close()

	err = fNS.Do(func(ns.NetNS) error {
		attrs := netlink.NewLinkAttrs()
		attrs.Name = "dummy1"
		attrs.Flags = net.FlagUp
		if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
			return err
		}
		dummy, err := netlink.LinkByName("dummy1")
		if err != nil {
			return err
		}
		err = netlink.AddrAdd(dummy, &netlink.Addr{
			IPNet: &net.IPNet{IP: net.ParseIP("fd02::100"), Mask: net.CIDRMask(120, 128)},
		})
		if err != nil {
			return err
		}

		fou := NewClientFoUTunnel(5555, nil, net.ParseIP("::1"), nil)
		if fou.IsInitialized() {
			return errors.New("expect not to be initialized, but it's already been done")
		}

		if err := fou.Init(); err != nil {
			return fmt.Errorf("fou.Init failed: %w", err)
		}

		if !fou.IsInitialized() {
			return errors.New("expect to be initialized, but it's not been done")
		}

		fous, err := netlink.FouList(0)
		if err != nil {
			return fmt.Errorf("failed to list fou links: %w", err)
		}
		if len(fous) != 1 {
			return fmt.Errorf("unexpected fou list: %+v", fous)
		}
		for i, f := range fous {
			if f.Port != 5555 {
				return fmt.Errorf("unexpected fous[%d] port number: %d", i, f.Port)
			}
		}

		if _, err := fou.AddPeer(net.ParseIP("10.1.1.1"), true); err != ErrIPFamilyMismatch {
			return fmt.Errorf("error is not ErrIPFamilyMismatch: %w", err)
		}

		if link, err := fou.AddPeer(net.ParseIP("fd02::101"), true); err != nil {
			return fmt.Errorf("failed to call AddPeer with fd02::101: %w", err)
		} else {
			ip6tnl, ok := link.(*netlink.Ip6tnl)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !ip6tnl.Remote.Equal(net.ParseIP("fd02::101")) {
				return fmt.Errorf("remote is not fd02::101: %s", ip6tnl.Remote.String())
			}
			if !ip6tnl.Local.Equal(net.ParseIP("::1")) {
				return fmt.Errorf("local is not ::1: %s", ip6tnl.Local.String())
			}
			if ip6tnl.EncapDport != 5555 {
				return fmt.Errorf("ip6tnl.EncapDport is not 5555: %d", ip6tnl.EncapDport)
			}
			if ip6tnl.EncapSport != 0 {
				return fmt.Errorf("ip6tnl.EncapSport is not 0: %d", ip6tnl.EncapSport)
			}

			ipip6, err := netlink.LinkByName("coil_ipip6")
			if err != nil {
				return fmt.Errorf("failed to get coil_ipip6: %w", err)
			}
			ip6tnl, ok = ipip6.(*netlink.Ip6tnl)
			if !ok {
				return fmt.Errorf("link is not Iptun: %T", link)
			}
			if !ip6tnl.FlowBased {
				return errors.New("coil_ipip6 is not flow based")
			}
		}

		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func testFoUServer(t *testing.T) {
	t.Parallel()

	fNS, err := testutils.NewNS()
	if err != nil {
		t.Fatal(err)
	}
	defer fNS.Close()

	err = fNS.Do(func(ns.NetNS) error {
		fou := NewServerFoUTunnel(5555, net.ParseIP("127.0.0.1"), net.ParseIP("::1"), nil)
		if err := fou.Init(); err != nil {
			return fmt.Errorf("fou.Init failed: %w", err)
		}

		// NAT servers forward packets from clients
		if err := expectSysctl("net.ipv4.ip_forward", "1"); err != nil {
			return err
		}
		if err := expectSysctl("net.ipv6.conf.all.forwarding", "1"); err != nil {
			return err
		}

		if _, err := fou.AddPeer(net.ParseIP("10.1.1.1"), true); err != nil {
			return fmt.Errorf("failed to call AddPeer with 10.1.1.1: %w", err)
		}
		if _, err := fou.AddPeer(net.ParseIP("fd02::101"), true); err != nil {
			return fmt.Errorf("failed to call AddPeer with fd02::101: %w", err)
		}

		// NAT servers must not accept packets from unknown peers
		for _, name := range []string{"coil_ipip4", "coil_ipip6"} {
			_, err := netlink.LinkByName(name)
			if err == nil {
				return fmt.Errorf("unexpected flow based device: %s", name)
			}
			if !errors.As(err, new(netlink.LinkNotFoundError)) {
				return fmt.Errorf("failed to get %s: %w", name, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func expectSysctl(key, want string) error {
	got, err := sysctl.Sysctl(key)
	if err != nil {
		return fmt.Errorf("failed to get %s: %w", key, err)
	}
	if got != want {
		return fmt.Errorf("unexpected %s: got %s, want %s", key, got, want)
	}
	return nil
}
