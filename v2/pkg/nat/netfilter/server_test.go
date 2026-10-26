//go:build privileged

package netfilter

import (
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/coreos/go-iptables/iptables"
	"github.com/google/nftables"
	"github.com/vishvananda/netlink"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nat"
)

var (
	errNoEgressRule = errors.New("no egress rule found")
)

func TestNewNat(t *testing.T) {
	iface := "lo"
	ipv4 := net.ParseIP("127.0.0.1")
	ipv6 := net.ParseIP("::1")

	type args struct {
		iface   string
		ipv4    net.IP
		ipv6    net.IP
		backend string
	}
	tests := []struct {
		name    string
		args    args
		want    *NatServer
		wantErr bool
	}{
		{
			name: "IPv4 and IPv6 with iptables",
			args: args{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    ipv6,
				backend: constants.EgressBackendIPTables,
			},
			want: &NatServer{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    ipv6,
				backend: constants.EgressBackendIPTables,
				clients: make(map[string]string),
			},
			wantErr: false,
		}, {
			name: "IPv4 and IPv6 with nftables",
			args: args{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    ipv6,
				backend: constants.EgressBackendNFTables,
			},
			want: &NatServer{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    ipv6,
				backend: constants.EgressBackendNFTables,
				clients: make(map[string]string),
			},
			wantErr: false,
		},
		{
			name: "IPv4 with iptables",
			args: args{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    nil,
				backend: constants.EgressBackendIPTables,
			},
			want: &NatServer{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    nil,
				backend: constants.EgressBackendIPTables,
				clients: make(map[string]string),
			},
			wantErr: false,
		},
		{
			name: "IPv4 with nftables",
			args: args{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    nil,
				backend: constants.EgressBackendNFTables,
			},
			want: &NatServer{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    nil,
				backend: constants.EgressBackendNFTables,
				clients: make(map[string]string),
			},
			wantErr: false,
		},
		{
			name: "IPv6 with iptables",
			args: args{
				iface:   iface,
				ipv4:    nil,
				ipv6:    ipv6,
				backend: constants.EgressBackendIPTables,
			},
			want: &NatServer{
				iface:   iface,
				ipv4:    nil,
				ipv6:    ipv6,
				backend: constants.EgressBackendIPTables,
				clients: make(map[string]string),
			},
			wantErr: false,
		},
		{
			name: "IPv6 with nftables",
			args: args{
				iface:   iface,
				ipv4:    nil,
				ipv6:    ipv6,
				backend: constants.EgressBackendNFTables,
			},
			want: &NatServer{
				iface:   iface,
				ipv4:    nil,
				ipv6:    ipv6,
				backend: constants.EgressBackendNFTables,
				clients: make(map[string]string),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tns, err := testutils.NewNS()
			if err != nil {
				t.Error(err)
			}
			defer tns.Close()

			if err := tns.Do(func(ns ns.NetNS) error {
				got, err := NewNatServer(tt.args.iface, tt.args.ipv4, tt.args.ipv6, tt.args.backend)
				if (err != nil) != tt.wantErr {
					return fmt.Errorf("NewNatServer() error = %v, wantErr %v", err, tt.wantErr)
				}

				if !reflect.DeepEqual(got, tt.want) {
					return fmt.Errorf("NewNatServer() got = %v, want %v", got, tt.want)
				}

				switch tt.args.backend {
				case constants.EgressBackendIPTables:
					if err := checkIptables(tt.args.iface, tt.args.ipv4, tt.args.ipv6); err != nil {
						return err
					}
				case constants.EgressBackendNFTables:
					if err := checkNftables(tt.args.ipv4, tt.args.ipv6); err != nil {
						return err
					}
				}

				if err := checkIPRules(tt.args.iface, tt.args.ipv4, tt.args.ipv6); err != nil {
					return err
				}

				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func checkIptables(iface string, ipv4, ipv6 net.IP) error {
	if ipv4 == nil {
		if err := checkIptablesRules(iface, "", iptables.ProtocolIPv4); err != nil {
			return fmt.Errorf("ipv4 checkIptables error = %v", err)
		}
	} else {
		ipv4Str := ipv4.String() + "/32"
		if err := checkIptablesRules(iface, ipv4Str, iptables.ProtocolIPv4); err != nil {
			return fmt.Errorf("ipv4 checkIptables error = %v", err)
		}
	}

	if ipv6 == nil {
		if err := checkIptablesRules(iface, "", iptables.ProtocolIPv6); err != nil {
			return fmt.Errorf("ipv6 checkIptables error = %v", err)
		}
	} else {
		ipv6Str := ipv6.String() + "/128"
		if err := checkIptablesRules(iface, ipv6Str, iptables.ProtocolIPv6); err != nil {
			return fmt.Errorf("ipv6 checkIptables error = %v", err)
		}
	}
	return nil
}

func checkIptablesRules(iface string, ip string, family iptables.Protocol) error {
	ipt, err := iptables.NewWithProtocol(family)
	if err != nil {
		return err
	}

	// check masquerade rule
	src := ip
	if src == "" {
		switch family {
		case iptables.ProtocolIPv4:
			src = "127.0.0.1/32"
		case iptables.ProtocolIPv6:
			src = "::1/128"
		}
	}

	exist, err := ipt.Exists(natTable, natChain, "!", "-s", src, "-o", iface, "-j", "MASQUERADE")
	if err != nil {
		return err
	}
	if ip == "" && exist {
		return errors.New("unexpected NAT rule found")
	}
	if ip != "" && !exist {
		return errors.New("NAT rule not found")
	}

	// check drop invalid packets rule
	exist, err = ipt.Exists(filterTable, filterChain, "-o", iface, "-m", "state", "--state", "INVALID", "-m", "comment", "--comment", RuleTagInvalidDrop, "-j", "DROP")
	if err != nil {
		return err
	}
	if ip == "" && exist {
		return errors.New("unexpected DROP rule found")
	}
	if ip != "" && !exist {
		return errors.New("DROP rule not found")
	}

	return nil
}

func checkNftables(ipv4, ipv6 net.IP) error {
	if err := checkNftablesRules(ipv4, nftables.TableFamilyIPv4); err != nil {
		return fmt.Errorf("ipv4 checkNftables error = %v", err)
	}

	if err := checkNftablesRules(ipv6, nftables.TableFamilyIPv6); err != nil {
		return fmt.Errorf("ipv6 checkNftables error = %v", err)
	}

	return nil
}

func checkNftablesRules(ip net.IP, family nftables.TableFamily) error {
	conn, err := nftables.New()
	if err != nil {
		return err
	}

	t := &nftables.Table{Family: family, Name: natTable}
	c := &nftables.Chain{Name: natChain, Table: t}
	r, err := conn.GetRules(t, c)
	if err != nil {
		return fmt.Errorf("failed to get  NAT rules: %w", err)
	}

	if ip == nil && len(r) != 0 {
		return errors.New("unexpected NAT rules found for nil IP")
	}
	if ip != nil && len(r) == 0 {
		return errors.New("NAT rules not found")
	}
	return nil
}

func checkIPRules(iface string, ipv4, ipv6 net.IP) error {
	ipFamilyToIP := map[int]net.IP{
		netlink.FAMILY_V4: ipv4,
		netlink.FAMILY_V6: ipv6,
	}
	for f, ip := range ipFamilyToIP {
		r, err := getNatIPRule(f)
		if err != nil {
			if !errors.Is(err, errNoEgressRule) {
				return fmt.Errorf("failed to get NAT rule for : %w", err)
			}
			if ip != nil {
				return fmt.Errorf("no egress rule found for %s", ip.String())
			}
			// skip nil IP
			continue
		}

		if r.Table != nsTableID {
			return fmt.Errorf("wrong table for %s : %d", ip.String(), r.Table)
		}
		if r.IifName != iface {
			return fmt.Errorf("wrong incoming interface for %s: %s", ip.String(), r.IifName)
		}
	}
	return nil
}

func getNatIPRule(family int) (*netlink.Rule, error) {
	rs, err := netlink.RuleList(family)
	if err != nil {
		return nil, err
	}

	for _, r := range rs {
		if r.Priority == nsRulePrio {
			return &r, nil
		}
	}
	return nil, errNoEgressRule
}

func TestNat_AddClient(t *testing.T) {
	iface := "lo"
	ipv4 := net.ParseIP("127.0.0.1")
	ipv6 := net.ParseIP("::1")
	backend := constants.EgressBackendIPTables

	type fields struct {
		iface   string
		ipv4    net.IP
		ipv6    net.IP
		backend string
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "Dual stack",
			fields: fields{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    ipv6,
				backend: backend,
			},
		}, {
			name: "IPv4 Single stack",
			fields: fields{
				iface:   iface,
				ipv4:    ipv4,
				ipv6:    nil,
				backend: backend,
			},
		}, {
			name: "IPv6 Single stack",
			fields: fields{
				iface:   iface,
				ipv4:    nil,
				ipv6:    ipv6,
				backend: backend,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tns, err := testutils.NewNS()
			if err != nil {
				t.Error(err)
			}

			if err := tns.Do(func(ns ns.NetNS) error {
				n, err := NewNatServer(tt.fields.iface, tt.fields.ipv4, tt.fields.ipv6, tt.fields.backend)
				if err != nil {
					return fmt.Errorf("NewNatServer() error = %w", err)
				}

				attrs := netlink.NewLinkAttrs()
				attrs.Name = "dummy1"
				attrs.Flags = net.FlagUp
				if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
					return err
				}
				l, err := netlink.LinkByName("dummy1")
				if err != nil {
					return err
				}

				ips := []net.IP{net.ParseIP("10.1.2.3"), net.ParseIP("fd02::1")}
				// to check idempotency, call AddClient twice
				for i := 0; i < 2; i++ {
					for _, ip := range ips {
						err := n.AddClient(ip, l)
						if err == nil {
							continue
						}

						if !errors.Is(err, nat.ErrIPFamilyMismatch) {
							return fmt.Errorf("failed to call AddClient: %w", err)
						}
						if isIPv4(ip) && n.ipv4 != nil || !isIPv4(ip) && n.ipv6 != nil {
							return fmt.Errorf("unexpected error: %w", err)
						}
					}
				}

				if err := checkIPRoutes(tt.fields.ipv4, tt.fields.ipv6); err != nil {
					return err
				}

				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func checkIPRoutes(ipv4, ipv6 net.IP) error {
	ipFamilyToIP := map[int]net.IP{
		netlink.FAMILY_V4: ipv4,
		netlink.FAMILY_V6: ipv6,
	}
	for f, ip := range ipFamilyToIP {
		r, err := netlink.RouteListFiltered(f, &netlink.Route{Table: nsTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}

		if ip == nil && len(r) > 0 {
			return fmt.Errorf("unexpected route :%v", r)
		}
		if ip != nil && len(r) == 0 {
			return fmt.Errorf("no routes found for %s", ip.String())
		}
	}
	return nil
}

func TestNat_PeerSrcRule(t *testing.T) {
	ipv4 := net.ParseIP("127.0.0.1")
	ipv6 := net.ParseIP("::1")

	clients := []struct {
		family   int
		ip       net.IP
		linkName string
	}{
		{family: netlink.FAMILY_V4, ip: net.ParseIP("10.1.2.3"), linkName: "fou4_test"},
		{family: netlink.FAMILY_V6, ip: net.ParseIP("fd02::1"), linkName: "fou6_test"},
	}

	for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
		t.Run(backend, func(t *testing.T) {
			t.Parallel()

			tns, err := testutils.NewNS()
			if err != nil {
				t.Fatal(err)
			}
			defer tns.Close()

			if err := tns.Do(func(ns ns.NetNS) error {
				n, err := NewNatServer("lo", ipv4, ipv6, backend)
				if err != nil {
					return fmt.Errorf("NewNatServer() error = %w", err)
				}

				links := make(map[string]netlink.Link)
				for _, c := range clients {
					attrs := netlink.NewLinkAttrs()
					attrs.Name = c.linkName
					if err := netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs}); err != nil {
						return err
					}
					l, err := netlink.LinkByName(c.linkName)
					if err != nil {
						return err
					}
					links[c.linkName] = l
				}

				// to check idempotency, call AddClient twice
				for i := 0; i < 2; i++ {
					for _, c := range clients {
						if err := n.AddClient(c.ip, links[c.linkName]); err != nil {
							return fmt.Errorf("failed to call AddClient for %s: %w", c.ip, err)
						}
					}
				}
				for _, c := range clients {
					if err := expectPeerSrcRules(backend, c.family, c.linkName, c.ip, 1); err != nil {
						return err
					}
					// the metrics collector finds the rule for invalid packets by its tag
					if err := expectInvalidRuleTagged(backend, c.family); err != nil {
						return err
					}
				}

				// AddClient restores the rule even if the route already exists
				c := clients[0]
				if err := removePeerSrcRule(backend, c.family, c.linkName, c.ip); err != nil {
					return err
				}
				if err := expectPeerSrcRules(backend, c.family, c.linkName, c.ip, 0); err != nil {
					return err
				}
				if err := n.AddClient(c.ip, links[c.linkName]); err != nil {
					return fmt.Errorf("failed to call AddClient for %s: %w", c.ip, err)
				}
				if err := expectPeerSrcRules(backend, c.family, c.linkName, c.ip, 1); err != nil {
					return err
				}

				// to check idempotency, call DelClient twice
				for i := 0; i < 2; i++ {
					for _, c := range clients {
						if err := n.DelClient(c.ip); err != nil {
							return fmt.Errorf("failed to call DelClient for %s: %w", c.ip, err)
						}
					}
				}
				for _, c := range clients {
					if err := expectPeerSrcRules(backend, c.family, c.linkName, c.ip, 0); err != nil {
						return err
					}
				}
				if got := n.GetClients(); len(got) != 0 {
					return fmt.Errorf("unexpected clients: %v", got)
				}
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func removePeerSrcRule(backend string, family int, linkName string, ip net.IP) error {
	switch backend {
	case constants.EgressBackendIPTables:
		return removeIPTablesPeerSrcRule(family, linkName, ip)
	default:
		return removeNFTablesPeerSrcRule(family, linkName)
	}
}

func expectPeerSrcRules(backend string, family int, linkName string, ip net.IP, want int) error {
	chains := []struct {
		name string
		nft  func(nftables.TableFamily) (*nftables.Table, *nftables.Chain)
	}{
		{filterChain, nftFilterForwardChain},
		{inputChain, nftFilterInputChain},
	}

	for _, chain := range chains {
		var got int
		switch backend {
		case constants.EgressBackendIPTables:
			ipt, err := newIPTables(family)
			if err != nil {
				return err
			}
			rules, err := ipt.List(filterTable, chain.name)
			if err != nil {
				return err
			}
			for _, r := range rules {
				if strings.Contains(r, "-i "+linkName+" ") {
					got++
				}
			}
			if want > 0 {
				exist, err := ipt.Exists(filterTable, chain.name, peerSrcRuleSpec(linkName, ip)...)
				if err != nil {
					return err
				}
				if !exist {
					return fmt.Errorf("peer source rule for %s not found in %s: %v", linkName, chain.name, rules)
				}
			}
		default:
			rules, err := listNFTablesFilterRules(family, chain.nft)
			if err != nil {
				return err
			}
			for _, r := range rules {
				if string(r.UserData) == RuleTagPeerSrcPrefix+linkName {
					got++
				}
			}
		}

		if got != want {
			return fmt.Errorf("unexpected number of peer source rules for %s in %s: got %d, want %d", linkName, chain.name, got, want)
		}
	}
	return nil
}

func expectInvalidRuleTagged(backend string, family int) error {
	var got int
	switch backend {
	case constants.EgressBackendIPTables:
		ipt, err := newIPTables(family)
		if err != nil {
			return err
		}
		rules, err := ipt.List(filterTable, filterChain)
		if err != nil {
			return err
		}
		for _, r := range rules {
			if strings.Contains(r, "INVALID") && strings.Contains(r, "--comment "+RuleTagInvalidDrop) {
				got++
			}
		}
	default:
		rules, err := listNFTablesForwardRules(family)
		if err != nil {
			return err
		}
		for _, r := range rules {
			if string(r.UserData) == RuleTagInvalidDrop {
				got++
			}
		}
	}
	if got != 1 {
		return fmt.Errorf("unexpected number of tagged rules for invalid packets: got %d, want 1", got)
	}
	return nil
}

func listNFTablesForwardRules(family int) ([]*nftables.Rule, error) {
	return listNFTablesFilterRules(family, nftFilterForwardChain)
}

func listNFTablesFilterRules(family int, chain func(nftables.TableFamily) (*nftables.Table, *nftables.Chain)) ([]*nftables.Rule, error) {
	conn, err := nftables.New()
	if err != nil {
		return nil, err
	}
	nf, err := netlinkToNFTablesFamily(family)
	if err != nil {
		return nil, err
	}
	t, c := chain(nf)
	return conn.GetRules(t, c)
}
