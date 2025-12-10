//go:build privileged

package netfilter

import (
	"bytes"
	"fmt"
	"net"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/coreos/go-iptables/iptables"
	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
)

const (
	testDummyDev = "dummy-nat"
)

func TestNewNatClient(t *testing.T) {
	ipv4 := net.ParseIP("10.1.1.1")
	ipv6 := net.ParseIP("fd02::1")
	podNodeNet := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	v4InCluster := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
	}
	v6InCluster := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	backend := BackendIptables

	type args struct {
		ipv4       net.IP
		ipv6       net.IP
		podNodeNet []*net.IPNet
		backend    string
		logFunc    func(string)
	}
	tests := []struct {
		name string
		args args
		want *NatClient
	}{
		{
			name: "IPv4 and IPv6",
			args: args{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			want: &NatClient{
				ipv4:        ipv4,
				ipv6:        ipv6,
				v4InCluster: v4PrivateList,
				v6InCluster: v6PrivateList,
				backend:     backend,
				logFunc:     nil,
			},
		},
		{
			name: "IPv4",
			args: args{
				ipv4:       ipv4,
				ipv6:       nil,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			want: &NatClient{
				ipv4:        ipv4,
				ipv6:        nil,
				v4InCluster: v4PrivateList,
				v6InCluster: v6PrivateList,
				backend:     backend,
				logFunc:     nil,
			},
		},
		{
			name: "IPv6",
			args: args{
				ipv4:       nil,
				ipv6:       ipv6,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			want: &NatClient{
				ipv4:        nil,
				ipv6:        ipv6,
				v4InCluster: v4PrivateList,
				v6InCluster: v6PrivateList,
				backend:     backend,
				logFunc:     nil,
			},
		},
		{
			name: "With podNodeNet",
			args: args{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: podNodeNet,
				backend:    backend,
				logFunc:    nil,
			},
			want: &NatClient{
				ipv4:        ipv4,
				ipv6:        ipv6,
				v4InCluster: v4InCluster,
				v6InCluster: v6InCluster,
				backend:     backend,
				logFunc:     nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NewNatClient(tt.args.ipv4, tt.args.ipv6, tt.args.podNodeNet, tt.args.backend, tt.args.logFunc)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewNatClient() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNatClient_Init(t *testing.T) {
	ipv4 := net.ParseIP("10.1.1.1")
	ipv6 := net.ParseIP("fd02::1")
	podNodeNet := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	backend := BackendIptables

	type ncArgs struct {
		ipv4       net.IP
		ipv6       net.IP
		podNodeNet []*net.IPNet
		backend    string
		logFunc    func(string)
	}
	tests := []struct {
		name    string
		args    *ncArgs
		wantErr bool
	}{
		{
			name: "IPv4 and IPv6",
			args: &ncArgs{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "IPv4",
			args: &ncArgs{
				ipv4:       ipv4,
				ipv6:       nil,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "IPv6",
			args: &ncArgs{
				ipv4:       nil,
				ipv6:       ipv6,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "With podNodeNet",
			args: &ncArgs{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: podNodeNet,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tns, err := testutils.NewNS()
			if err != nil {
				t.Fatal(err)
			}
			defer tns.Close()

			if err := tns.Do(func(ns ns.NetNS) error {
				nc := NewNatClient(tt.args.ipv4, tt.args.ipv6, tt.args.podNodeNet, tt.args.backend, tt.args.logFunc)

				if err := nc.Init(); (err != nil) != tt.wantErr {
					return fmt.Errorf("Init() error = %w, wantErr %v", err, tt.wantErr)
				}

				if err := checkInitRules(nc); err != nil {
					return fmt.Errorf("NewNatClient() initialization error = %v", err)
				}
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestNatClient_IsInitialized(t *testing.T) {
	ipv4 := net.ParseIP("10.1.1.1")
	ipv6 := net.ParseIP("fd02::1")
	podNodeNet := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	backend := BackendIptables

	type ncArgs struct {
		ipv4       net.IP
		ipv6       net.IP
		podNodeNet []*net.IPNet
		backend    string
		logFunc    func(string)
	}

	tests := []struct {
		name    string
		args    *ncArgs
		wantErr bool
	}{
		{
			name: "IPv4 and IPv6",
			args: &ncArgs{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "IPv4",
			args: &ncArgs{
				ipv4:       ipv4,
				ipv6:       nil,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "IPv6",
			args: &ncArgs{
				ipv4:       nil,
				ipv6:       ipv6,
				podNodeNet: nil,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "With podNodeNet",
			args: &ncArgs{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: podNodeNet,
				backend:    backend,
				logFunc:    nil,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tns, err := testutils.NewNS()
			if err != nil {
				t.Fatal(err)
			}
			defer tns.Close()

			if err := tns.Do(func(ns ns.NetNS) error {
				nc := NewNatClient(tt.args.ipv4, tt.args.ipv6, tt.args.podNodeNet, tt.args.backend, tt.args.logFunc)

				got, err := nc.IsInitialized()
				if (err != nil) != tt.wantErr {
					return fmt.Errorf("IsInitialized() error = %v, wantErr %v", err, tt.wantErr)
				}
				if got != false {
					t.Errorf("IsInitialized() got = %v, want %v", got, false)
				}

				if err := nc.Init(); err != nil {
					return fmt.Errorf("Init() error = %w", err)
				}

				got, err = nc.IsInitialized()
				if (err != nil) != tt.wantErr {
					return fmt.Errorf("IsInitialized() error = %v, wantErr %v", err, tt.wantErr)
				}
				if got != true {
					t.Errorf("IsInitialized() got = %v, want %v", got, true)
				}
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestNatClient_SyncNat(t *testing.T) {
	ipv4 := net.ParseIP("10.1.1.1")
	ipv6 := net.ParseIP("fd02::1")
	podNodeNet := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	v4SubnetsFirst := []*net.IPNet{
		{IP: net.ParseIP("10.1.2.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
	}
	v4SubnetsSecond := []*net.IPNet{
		{IP: net.ParseIP("10.1.3.0"), Mask: net.CIDRMask(24, 32)},
		{IP: net.ParseIP("9.9.9.9"), Mask: net.CIDRMask(32, 32)},
		{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)},
	}
	v6SubnetsFirst := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
	}
	v6SubnetsSecond := []*net.IPNet{
		{IP: net.ParseIP("fd03::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("fd04::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("fd05::"), Mask: net.CIDRMask(64, 128)},
		{IP: net.ParseIP("::"), Mask: net.CIDRMask(0, 128)},
	}

	type fields struct {
		ipv4       net.IP
		ipv6       net.IP
		podNodeNet []*net.IPNet
		logFunc    func(string)
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "IPv4 and IPv6",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: nil,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "IPv4",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       nil,
				podNodeNet: nil,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "IPv6",
			fields: fields{
				ipv4:       nil,
				ipv6:       ipv6,
				podNodeNet: nil,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "With podNodeNet",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: podNodeNet,
				logFunc:    nil,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: t.Parallel() is disabled because the google/nftables library
			// has a race condition in AddSet() when multiple tests run concurrently.
			// This is a known issue in the library's global state management.

			for _, backend := range []string{BackendIptables, BackendNftables} {
				for _, originatingOnly := range []bool{true, false} {
					tns, err := testutils.NewNS()
					if err != nil {
						t.Fatal(err)
					}
					defer tns.Close()

					if err := tns.Do(func(ns ns.NetNS) error {
						nc := NewNatClient(tt.fields.ipv4, tt.fields.ipv6, tt.fields.podNodeNet, backend, tt.fields.logFunc)

						if err := nc.Init(); err != nil {
							return fmt.Errorf("Init() error = %w", err)
						}

						link, err := newDummyDevice(testDummyDev)
						if err != nil {
							return fmt.Errorf("failed to create dummy device: %w", err)
						}

						// First sync
						subnets := append(v4SubnetsFirst, v6SubnetsFirst...)
						if err := nc.SyncNat(link, subnets, originatingOnly); (err != nil) != tt.wantErr {
							return fmt.Errorf("SyncNat() error = %v, wantErr %v", err, tt.wantErr)
						}

						if err := checkNatClientRoutes(nc, v4SubnetsFirst, v6SubnetsFirst); err != nil {
							return fmt.Errorf("NatClient routes check failed: %w", err)
						}

						// Second sync (check for diff handling)
						subnets = append(v4SubnetsSecond, v6SubnetsSecond...)
						if err := nc.SyncNat(link, subnets, originatingOnly); (err != nil) != tt.wantErr {
							return fmt.Errorf("SyncNat() error = %v, wantErr %v", err, tt.wantErr)
						}

						if err := checkNatClientRoutes(nc, v4SubnetsSecond, v6SubnetsSecond); err != nil {
							return fmt.Errorf("NatClient routes check failed: %w", err)
						}

						// check originatingOnly rules
						if originatingOnly {
							if err := checkOriginatingOnlyRules(nc); err != nil {
								return fmt.Errorf("OriginatingOnly rules check failed: %w", err)
							}
						}

						// NATClient can be re-initialized
						if err := nc.Init(); err != nil {
							return fmt.Errorf("failed to re-initialize NATClient: %w", err)
						}

						if err := checkNatClientRoutesCleared(nc); err != nil {
							return fmt.Errorf("NatClient routes cleared failed: %w", err)
						}
						return nil
					}); err != nil {
						t.Error(err)
					}
				}
			}
		})
	}
}

func checkInitRules(nc *NatClient) error {
	if nc.ipv4 != nil {
		if err := checkInitRulesForFamily(netlink.FAMILY_V4, nc.v4InCluster); err != nil {
			return fmt.Errorf("IPv4: %w", err)
		}
	}
	if nc.ipv6 != nil {
		if err := checkInitRulesForFamily(netlink.FAMILY_V6, nc.v6InCluster); err != nil {
			return fmt.Errorf("IPv6: %w", err)
		}
	}
	return nil
}

func checkInitRulesForFamily(family int, inCluster []*net.IPNet) error {
	rm, err := ruleMap(family)
	if err != nil {
		return err
	}

	// Check fixed rules
	expectedRules := map[int]int{
		ncLinkLocalPrio: mainTableID,
		ncNarrowPrio:    ncNarrowTableID,
		ncWidePrio:      ncWideTableID,
	}

	for prio, expectedTable := range expectedRules {
		r, ok := rm[prio]
		if !ok {
			return fmt.Errorf("no rule at priority %d", prio)
		}
		if r.Table != expectedTable {
			return fmt.Errorf("rule at priority %d should point to table %d, got %d", prio, expectedTable, r.Table)
		}
	}

	// Check inCluster rules
	for i := range inCluster {
		prio := ncLocalPrioBase + i
		r, ok := rm[prio]
		if !ok {
			return fmt.Errorf("no inCluster rule at priority %d", prio)
		}
		if r.Table != mainTableID {
			return fmt.Errorf("inCluster rule at priority %d should point to main table %d, got %d", prio, mainTableID, r.Table)
		}
	}

	return nil
}

func checkNatClientRoutes(nc *NatClient, v4Nets, v6Nets []*net.IPNet) error {
	if err := checkNatClientRoutesByFamily(nc.ipv4, nc.v4InCluster, v4Nets, netlink.FAMILY_V4); err != nil {
		return fmt.Errorf("IPv4: %w", err)
	}
	if err := checkNatClientRoutesByFamily(nc.ipv6, nc.v6InCluster, v6Nets, netlink.FAMILY_V6); err != nil {
		return fmt.Errorf("IPv6: %w", err)
	}
	return nil
}

func checkNatClientRoutesByFamily(clientIP net.IP, inCluster, subnets []*net.IPNet, family int) error {
	subenetsByTable := map[int][]*net.IPNet{}
	for _, sn := range subnets {
		if slices.ContainsFunc(inCluster, func(n *net.IPNet) bool {
			return n.Contains(sn.IP)
		}) {
			subenetsByTable[ncNarrowTableID] = append(subenetsByTable[ncNarrowTableID], sn)
		} else {
			subenetsByTable[ncWideTableID] = append(subenetsByTable[ncWideTableID], sn)
		}
	}

	for _, table := range []int{ncNarrowTableID, ncWideTableID} {
		rs, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return err
		}

		// if clientIP is nil, there should be no routes in the table
		if clientIP == nil {
			if len(rs) == 0 {
				continue
			} else {
				return fmt.Errorf("client should not have routes in table %d", table)
			}
		}

		if len(subenetsByTable[table]) != len(rs) {
			return fmt.Errorf("failed to sync routes in table %d", table)
		}

		routesMap := make(map[string]struct{})
		for _, r := range rs {
			routesMap[r.Dst.IP.String()] = struct{}{}
		}

		for _, sn := range subenetsByTable[table] {
			if _, ok := routesMap[sn.IP.String()]; !ok {
				return fmt.Errorf("missing route %s in table %d", sn.IP.String(), table)
			}
		}
	}
	return nil
}

func checkOriginatingOnlyRules(nc *NatClient) error {
	link, err := netlink.LinkByName(testDummyDev)
	if err != nil {
		return fmt.Errorf("failed to get dummy device: %w", err)
	}

	if nc.ipv4 != nil {
		if err := checkOriginatingOnlyFWMarkRule(link, netlink.FAMILY_V4); err != nil {
			return fmt.Errorf("IPv4 FWMark rules check failed: %w", err)
		}
		if err := checkOriginatingOnlyFWMarkTable(link, netlink.FAMILY_V4); err != nil {
			return fmt.Errorf("IPv4 FWMark table routes check failed: %w", err)
		}
	}

	if nc.ipv6 != nil {
		if err := checkOriginatingOnlyFWMarkRule(link, netlink.FAMILY_V6); err != nil {
			return fmt.Errorf("IPv6 FWMark rules check failed: %w", err)
		}
		if err := checkOriginatingOnlyFWMarkTable(link, netlink.FAMILY_V6); err != nil {
			return fmt.Errorf("IPv6 FWMark table routes check failed: %w", err)
		}
	}

	switch nc.backend {
	case BackendIptables:
		if nc.ipv4 != nil {
			if err := checkOriginatingOnlyIPTablesRules(link, netlink.FAMILY_V4); err != nil {
				return fmt.Errorf("IPv4 IPTables rules check failed: %w", err)
			}
		}
		if nc.ipv6 != nil {
			if err := checkOriginatingOnlyIPTablesRules(link, netlink.FAMILY_V6); err != nil {
				return fmt.Errorf("IPv4 IPTables rules check failed: %w", err)
			}
		}
	case BackendNftables:
		if nc.ipv4 != nil {
			if err := checkOriginatingOnlyNFTablesRules(link, netlink.FAMILY_V4); err != nil {
				return fmt.Errorf("IPv4 NFTables rules check failed: %w", err)
			}
		}
		if nc.ipv6 != nil {
			if err := checkOriginatingOnlyNFTablesRules(link, netlink.FAMILY_V6); err != nil {
				return fmt.Errorf("IPv6 NFTables rules check failed: %w", err)
			}
		}
	}
	return nil
}

func checkOriginatingOnlyFWMarkRule(link netlink.Link, family int) error {
	table := nonEgressTableID + link.Attrs().Index

	// Check if the FWMark rule exists
	_, exists, err := checkFWMarkRule(link, table, family)
	if err != nil {
		return fmt.Errorf("failed to check FWMark rule: %w", err)
	}
	if !exists {
		return fmt.Errorf("FWMark rule for link %q (table %d, family %d) does not exist", link.Attrs().Name, table, family)
	}

	// Verify the rule details
	rules, err := netlink.RuleList(family)
	if err != nil {
		return fmt.Errorf("failed to list rules: %w", err)
	}

	for _, r := range rules {
		if r.Mark == uint32(link.Attrs().Index) && r.Table == table {
			if r.Priority != ncFWMarkPrio {
				return fmt.Errorf("FWMark rule priority mismatch: expected %d, got %d", ncFWMarkPrio, r.Priority)
			}
			return nil
		}
	}
	return fmt.Errorf("FWMark rule not found for link %q", link.Attrs().Name)
}

func checkOriginatingOnlyFWMarkTable(link netlink.Link, family int) error {
	table := nonEgressTableID + link.Attrs().Index

	mainRoutes, err := netlink.RouteList(link, family)
	if err != nil {
		return err
	}

	fwmarkRoutes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return err
	}

	if slices.CompareFunc(mainRoutes, fwmarkRoutes, func(o netlink.Route, n netlink.Route) int {
		if o.Dst.IP.Equal(n.Dst.IP) && o.Table != n.Table {
			return 0
		}
		return 1
	}) != 0 {
		return fmt.Errorf("FWMark table %d routes do not match main table routes (main: %d, fwmark: %d)",
			table, len(mainRoutes), len(fwmarkRoutes))
	}
	return nil
}

func checkOriginatingOnlyIPTablesRules(link netlink.Link, family int) error {
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
	if ok, err := ipt.Exists(mangleTable, inputChain, inputSpec...); err != nil {
		return fmt.Errorf("failed to check IPTables mangle INPUT rule: %w", err)
	} else if !ok {
		return fmt.Errorf("expected IPTables mangle INPUT rule does not exist")
	}

	outputSpec := []string{"-j", "CONNMARK", "-m", "connmark", "--mark", strconv.Itoa(link.Attrs().Index), "--restore-mark"}
	if ok, err := ipt.Exists(mangleTable, outputChain, outputSpec...); err != nil {
		return fmt.Errorf("failed to check IPTables mangle OUTPUT rule: %w", err)
	} else if !ok {
		return fmt.Errorf("expected IPTables mangle OUTPUT rule does not exist")
	}
	return nil
}

func checkOriginatingOnlyNFTablesRules(link netlink.Link, family int) error {
	nf, err := netlinkToNFTablesFamily(family)
	if err != nil {
		return err
	}

	conn, err := nftables.New()
	if err != nil {
		return err
	}

	// Use ListTables and filter by name, as ListTableOfFamily fails
	// in certain conditions (fresh namespaces or when table doesn't exist).
	tables, err := conn.ListTables()
	if err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}

	var table *nftables.Table
	for _, t := range tables {
		if t.Name == mangleTable && t.Family == nf {
			table = t
			break
		}
	}
	if table == nil {
		return fmt.Errorf("mangle table not found")
	}

	// Use ListChains and filter, as ListChain fails in certain conditions
	chains, err := conn.ListChains()
	if err != nil {
		return fmt.Errorf("failed to list chains: %w", err)
	}

	var input, output *nftables.Chain
	for _, c := range chains {
		if c.Table.Name == table.Name && c.Table.Family == table.Family {
			if c.Name == strings.ToLower(inputChain) {
				input = c
			} else if c.Name == outputChain {
				output = c
			}
		}
	}
	if input == nil {
		return fmt.Errorf("input chain not found in table %q", table.Name)
	}
	if output == nil {
		return fmt.Errorf("output chain not found in table %q", table.Name)
	}

	// Use GetSets and filter, as GetSetByName fails in certain conditions
	sets, err := conn.GetSets(table)
	if err != nil {
		return fmt.Errorf("failed to get sets: %w", err)
	}

	var deviceSet *nftables.Set
	for _, s := range sets {
		if s.Name == link.Attrs().Name {
			deviceSet = s
			break
		}
	}
	if deviceSet == nil {
		return fmt.Errorf("set for device %q not found", link.Attrs().Name)
	}

	markRule := &nftables.Rule{
		Table:    table,
		Chain:    input,
		UserData: []byte(nftRuleIDInputPrefix + link.Attrs().Name),
		Exprs: []expr.Any{
			&expr.Meta{
				Key:      expr.MetaKeyIIFNAME,
				Register: 1,
			},
			&expr.Lookup{
				SetName:        deviceSet.Name,
				SetID:          deviceSet.ID,
				SourceRegister: 1,
			},
			&expr.Ct{
				Register: 1,
				Key:      expr.CtKeySTATE,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitNEW | expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: 1,
				Data:     []byte{0, 0, 0, 0},
			},
			&expr.Counter{},
			&expr.Immediate{
				Register: 1,
				Data:     binaryutil.NativeEndian.PutUint32(uint32(link.Attrs().Index)),
			},
			&expr.Ct{
				Key:            expr.CtKeyMARK,
				SourceRegister: false,
				Register:       0,
			},
		},
	}

	rules, err := conn.GetRules(table, input)
	if err != nil {
		return fmt.Errorf("failed to get rules in table %q chain %q: %w", table.Name, input.Name, err)
	}

	hasSame := false
	for _, r := range rules {
		if same := compareNftRule(markRule, r); same {
			hasSame = true
		}
	}
	if !hasSame {
		return fmt.Errorf("expected conn mangle input rule does not exist")
	}

	restoreMarkRule := &nftables.Rule{
		Table:    table,
		Chain:    output,
		UserData: []byte(nftRuleIDOutputPrefix + link.Attrs().Name),
		Exprs: []expr.Any{
			&expr.Ct{
				Register: 1,
				Key:      expr.CtKeyMARK,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.NativeEndian.PutUint32(uint32(link.Attrs().Index)),
			},
			&expr.Counter{},
			&expr.Ct{
				Register: 1,
				Key:      expr.CtKeyMARK,
			},
			&expr.Meta{
				Key:            expr.MetaKeyMARK,
				SourceRegister: true,
				Register:       1,
			},
		},
	}

	rules, err = conn.GetRules(table, output)
	if err != nil {
		return fmt.Errorf("failed to get rules in table %q chain %q: %w", table.Name, output.Name, err)
	}
	for _, r := range rules {
		if same := compareNftRule(restoreMarkRule, r); same {
			return nil
		}
	}
	return fmt.Errorf("expected conn mangle output rule does not exist")
}

func checkNatClientRoutesCleared(nc *NatClient) error {
	familyStr := []string{"IPv4", "IPv6"}
	for i, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		for _, table := range []int{ncNarrowTableID, ncWideTableID} {
			routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
			if err != nil {
				return err
			}
			if len(routes) != 0 {
				return fmt.Errorf("routing table %d should be cleared for %s: %v", table, familyStr[i], routes)
			}
		}
	}
	return nil
}

func compareNftRule(src, dst *nftables.Rule) bool {
	if !bytes.Equal(src.UserData, dst.UserData) {
		return false
	}
	for i := range src.Exprs {
		// Ct exprs register returns a wrong value when SourceRegister is true,
		// so we skip them in comparison for now
		if _, ok := src.Exprs[i].(*expr.Ct); ok {
			continue
		}

		if reflect.TypeOf(src.Exprs[i]) != reflect.TypeOf(dst.Exprs[i]) {
			return false
		}
		if !reflect.DeepEqual(src.Exprs[i], dst.Exprs[i]) {
			return false
		}
	}
	return true
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

func newDummyDevice(name string) (netlink.Link, error) {
	attrs := netlink.NewLinkAttrs()
	attrs.Name = name
	attrs.Flags = net.FlagUp
	dummy := &netlink.Dummy{LinkAttrs: attrs}

	if err := netlink.LinkAdd(dummy); err != nil {
		return nil, fmt.Errorf("failed to add dummy link: %w", err)
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get dummy1: %w", err)
	}

	// Add global scope IPv4 address for originatingOnly tests
	ipv4Addr := &netlink.Addr{
		IPNet: &net.IPNet{IP: net.ParseIP("203.0.113.1"), Mask: net.CIDRMask(24, 32)},
		Scope: int(netlink.SCOPE_UNIVERSE),
	}
	if err := netlink.AddrAdd(link, ipv4Addr); err != nil {
		return nil, fmt.Errorf("failed to add IPv4 address: %w", err)
	}

	// Add global scope IPv6 address for originatingOnly tests
	ipv6Addr := &netlink.Addr{
		IPNet: &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
		Scope: int(netlink.SCOPE_UNIVERSE),
	}
	if err := netlink.AddrAdd(link, ipv6Addr); err != nil {
		return nil, fmt.Errorf("failed to add IPv6 address: %w", err)
	}

	// Add routes for the addresses
	if err := netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Scope:     netlink.SCOPE_HOST,
		Dst:       &net.IPNet{IP: net.ParseIP("203.0.113.1"), Mask: net.CIDRMask(32, 32)},
	}); err != nil {
		return nil, fmt.Errorf("failed to add IPv4 route: %w", err)
	}

	if err := netlink.RouteAdd(&netlink.Route{
		LinkIndex: link.Attrs().Index,
		Scope:     netlink.SCOPE_HOST,
		Dst:       &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(128, 128)},
	}); err != nil {
		return nil, fmt.Errorf("failed to add IPv6 route: %w", err)
	}

	return link, nil
}
