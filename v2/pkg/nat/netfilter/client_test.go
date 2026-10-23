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

	"github.com/cybozu-go/coil/v2/pkg/constants"
)

const (
	testDummyDev = "dummy-nat"
)

func TestNewNatClient(t *testing.T) {
	ipv4 := net.ParseIP("10.1.1.1")
	ipv6 := net.ParseIP("fd02::1")

	netsv4 := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
	}
	netsv6 := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	cNets := &ClusterNetworks{netsv4, netsv6}

	v4InCluster := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
	}
	v6InCluster := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	backend := constants.EgressBackendIPTables

	type args struct {
		ipv4    net.IP
		ipv6    net.IP
		cNets   *ClusterNetworks
		backend string
		logFunc func(string)
	}
	tests := []struct {
		name string
		args args
		want *NatClient
	}{
		{
			name: "IPv4 and IPv6",
			args: args{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			want: &NatClient{
				ipv4:      ipv4,
				ipv6:      ipv6,
				inCluster: &ClusterNetworks{v4PrivateList, v6PrivateList},
				backend:   backend,
				logFunc:   nil,
			},
		},
		{
			name: "IPv4",
			args: args{
				ipv4:    ipv4,
				ipv6:    nil,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			want: &NatClient{
				ipv4:      ipv4,
				ipv6:      nil,
				inCluster: &ClusterNetworks{v4PrivateList, v6PrivateList},
				backend:   backend,
				logFunc:   nil,
			},
		},
		{
			name: "IPv6",
			args: args{
				ipv4:    nil,
				ipv6:    ipv6,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			want: &NatClient{
				ipv4:      nil,
				ipv6:      ipv6,
				inCluster: &ClusterNetworks{v4PrivateList, v6PrivateList},
				backend:   backend,
				logFunc:   nil,
			},
		},
		{
			name: "With podNodeNet",
			args: args{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   cNets,
				backend: backend,
				logFunc: nil,
			},
			want: &NatClient{
				ipv4:      ipv4,
				ipv6:      ipv6,
				inCluster: &ClusterNetworks{v4InCluster, v6InCluster},
				backend:   backend,
				logFunc:   nil,
			},
		},
		{
			// Only an IPv6 override is given (e.g. a cluster using globally
			// routable IPv6 addresses). The IPv4 default must be untouched.
			name: "With only IPv6 override falls back to IPv4 default",
			args: args{
				ipv4: ipv4,
				ipv6: ipv6,
				cNets: &ClusterNetworks{
					[]*net.IPNet{}, cNets.v6,
				},
				backend: backend,
				logFunc: nil,
			},
			want: &NatClient{
				ipv4:      ipv4,
				ipv6:      ipv6,
				inCluster: &ClusterNetworks{v4PrivateList, v6InCluster},
				backend:   backend,
				logFunc:   nil,
			},
		},
		{
			// Only an IPv4 override is given. The IPv6 default must be untouched.
			name: "With only IPv4 override falls back to IPv6 default",
			args: args{
				ipv4: ipv4,
				ipv6: ipv6,
				cNets: &ClusterNetworks{
					cNets.v4, []*net.IPNet{},
				},
				backend: backend,
				logFunc: nil,
			},
			want: &NatClient{
				ipv4:      ipv4,
				ipv6:      ipv6,
				inCluster: &ClusterNetworks{v4InCluster, v6PrivateList},
				backend:   backend,
				logFunc:   nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewNatClient(tt.args.ipv4, tt.args.ipv6, tt.args.cNets, tt.args.backend, tt.args.logFunc)
			if err != nil {
				t.Errorf("NewNatClient() returned err = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewNatClient() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNatClient_Init(t *testing.T) {
	ipv4 := net.ParseIP("10.1.1.1")
	ipv6 := net.ParseIP("fd02::1")

	netsv4 := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
	}
	netsv6 := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	cNets := &ClusterNetworks{netsv4, netsv6}

	backend := constants.EgressBackendIPTables

	type ncArgs struct {
		ipv4    net.IP
		ipv6    net.IP
		cNets   *ClusterNetworks
		backend string
		logFunc func(string)
	}
	tests := []struct {
		name    string
		args    *ncArgs
		wantErr bool
	}{
		{
			name: "IPv4 and IPv6",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "IPv4",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    nil,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "IPv6",
			args: &ncArgs{
				ipv4:    nil,
				ipv6:    ipv6,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "With podNodeNet",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   cNets,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "With IPv6-only override",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   &ClusterNetworks{[]*net.IPNet{}, netsv6},
				backend: backend,
				logFunc: nil,
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
				nc, err := NewNatClient(tt.args.ipv4, tt.args.ipv6, tt.args.cNets, tt.args.backend, tt.args.logFunc)
				if err != nil {
					return fmt.Errorf("NewNatClient() returned err = %w", err)
				}
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

	netsv4 := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
	}
	netsv6 := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	cNets := &ClusterNetworks{netsv4, netsv6}

	backend := constants.EgressBackendIPTables

	type ncArgs struct {
		ipv4    net.IP
		ipv6    net.IP
		cNets   *ClusterNetworks
		backend string
		logFunc func(string)
	}

	tests := []struct {
		name    string
		args    *ncArgs
		wantErr bool
	}{
		{
			name: "IPv4 and IPv6",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "IPv4",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    nil,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "IPv6",
			args: &ncArgs{
				ipv4:    nil,
				ipv6:    ipv6,
				cNets:   nil,
				backend: backend,
				logFunc: nil,
			},
			wantErr: false,
		},
		{
			name: "With podNodeNet",
			args: &ncArgs{
				ipv4:    ipv4,
				ipv6:    ipv6,
				cNets:   cNets,
				backend: backend,
				logFunc: nil,
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
				nc, err := NewNatClient(tt.args.ipv4, tt.args.ipv6, tt.args.cNets, tt.args.backend, tt.args.logFunc)
				if err != nil {
					return fmt.Errorf("NewNatClient() returned err = %w", err)
				}

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

	netsv4 := []*net.IPNet{
		{IP: net.ParseIP("192.168.10.0"), Mask: net.CIDRMask(24, 32)},
	}
	netsv6 := []*net.IPNet{
		{IP: net.ParseIP("fd02::"), Mask: net.CIDRMask(16, 128)},
	}
	cNets := &ClusterNetworks{netsv4, netsv6}

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
		podNodeNet *ClusterNetworks
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
			name: "With cluster-networks",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: cNets,
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "With IPv4-only override",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: &ClusterNetworks{netsv4, []*net.IPNet{}},
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "With IPv6-only override",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: &ClusterNetworks{[]*net.IPNet{}, netsv6},
				logFunc:    nil,
			},
			wantErr: false,
		},
		{
			name: "With IPv4 and IPv6 override",
			fields: fields{
				ipv4:       ipv4,
				ipv6:       ipv6,
				podNodeNet: &ClusterNetworks{netsv4, netsv6},
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

			for _, backend := range []string{constants.EgressBackendIPTables, constants.EgressBackendNFTables} {
				for _, originatingOnly := range []bool{true, false} {
					tns, err := testutils.NewNS()
					if err != nil {
						t.Fatal(err)
					}
					defer tns.Close()

					if err := tns.Do(func(ns ns.NetNS) error {
						nc, err := NewNatClient(tt.fields.ipv4, tt.fields.ipv6, tt.fields.podNodeNet, backend, tt.fields.logFunc)
						if err != nil {
							return fmt.Errorf("NewNatClient() returned err = %w", err)
						}

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
	case constants.EgressBackendIPTables:
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
	case constants.EgressBackendNFTables:
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

	inputSpec := []string{
		"-m", "conntrack", "--ctstate", "NEW,ESTABLISHED,RELATED", "-j", "CONNMARK",
		"-i", link.Attrs().Name, "--set-mark", strconv.Itoa(link.Attrs().Index),
	}
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
			} else if c.Name == strings.ToLower(outputChain) {
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

func TestNatClient_MigrateRoutesWithClusterNetworks(t *testing.T) {
	tests := []struct {
		name       string
		family     int
		route      string
		link       string
		narrowCIDR string
		wideCIDR   string
		wantNarrow []routeTuple
		wantWide   []routeTuple
	}{
		{
			name:       "IPv4 public subnet",
			family:     netlink.FAMILY_V4,
			route:      "198.51.100.0/24",
			link:       "mig4-public",
			narrowCIDR: "198.51.0.0/16",
			wideCIDR:   "10.0.0.0/8",
			wantNarrow: []routeTuple{
				{netlink.FAMILY_V4, ncNarrowTableID, "198.51.100.0/24", "mig4-public"},
				{netlink.FAMILY_V4, ncWideTableID, "203.0.113.0/24", "mig4-public"},
			},
			wantWide: []routeTuple{
				{netlink.FAMILY_V4, ncWideTableID, "198.51.100.0/24", "mig4-public"},
				{netlink.FAMILY_V4, ncWideTableID, "203.0.113.0/24", "mig4-public"},
			},
		},
		{
			name:       "IPv6 public subnet",
			family:     netlink.FAMILY_V6,
			route:      "2001:db8:1::/64",
			link:       "mig6-public",
			narrowCIDR: "2001:db8::/32",
			wideCIDR:   "fd00::/8",
			wantNarrow: []routeTuple{
				{netlink.FAMILY_V6, ncNarrowTableID, "2001:db8:1::/64", "mig6-public"},
				{netlink.FAMILY_V6, ncWideTableID, "2001:db9::/64", "mig6-public"},
			},
			wantWide: []routeTuple{
				{netlink.FAMILY_V6, ncWideTableID, "2001:db8:1::/64", "mig6-public"},
				{netlink.FAMILY_V6, ncWideTableID, "2001:db9::/64", "mig6-public"},
			},
		},
		{
			name:       "IPv4 migrate default route when cluster networks change",
			family:     netlink.FAMILY_V4,
			route:      "0.0.0.0/0",
			link:       "mig4-default",
			narrowCIDR: "0.0.0.0/0",
			wideCIDR:   "10.0.0.0/8",
			wantNarrow: []routeTuple{
				{netlink.FAMILY_V4, ncNarrowTableID, "0.0.0.0/0", "mig4-default"},
			},
			wantWide: []routeTuple{
				{netlink.FAMILY_V4, ncWideTableID, "0.0.0.0/0", "mig4-default"},
			},
		},
		{
			name:       "IPv6 migrate default route when cluster networks change",
			family:     netlink.FAMILY_V6,
			route:      "::/0",
			link:       "mig6-default",
			narrowCIDR: "::/0",
			wideCIDR:   "fd00::/8",
			wantNarrow: []routeTuple{
				{netlink.FAMILY_V6, ncNarrowTableID, "::/0", "mig6-default"},
			},
			wantWide: []routeTuple{
				{netlink.FAMILY_V6, ncWideTableID, "::/0", "mig6-default"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tns := newRouteTestNS(t)
			client := newTestClient(t, nil)
			routes := []*net.IPNet{testCIDR(t, tt.route)}
			switch tt.route {
			case "198.51.100.0/24":
				routes = append(routes, testCIDR(t, "203.0.113.0/24"))
			case "2001:db8:1::/64":
				routes = append(routes, testCIDR(t, "2001:db9::/64"))
			}

			if err := tns.Do(func(ns.NetNS) error {
				link, err := newDummyLink(tt.link)
				if err != nil {
					return err
				}
				linkIndex := link.Attrs().Index
				if err := client.Init(); err != nil {
					return fmt.Errorf("initialize NatClient: %w", err)
				}
				if err := client.SyncNat(link, routes, false); err != nil {
					return fmt.Errorf("initial SyncNat: %w", err)
				}
				if err := assertRoutes(tt.wantWide); err != nil {
					return fmt.Errorf("initial wide routes: %w", err)
				}
				if err := assertRouteLinkIndexes(tt.wantWide, linkIndex); err != nil {
					return fmt.Errorf("initial route link indexes: %w", err)
				}

				if err := syncMigrationConfig(tt.family, tt.narrowCIDR, link, routes); err != nil {
					return fmt.Errorf("move route to narrow table: %w", err)
				}
				if err := assertRoutes(tt.wantNarrow); err != nil {
					return fmt.Errorf("routes after move to narrow table: %w", err)
				}
				if err := assertRouteLinkIndexes(tt.wantNarrow, linkIndex); err != nil {
					return fmt.Errorf("narrow route link indexes: %w", err)
				}
				if err := syncMigrationConfig(tt.family, tt.narrowCIDR, link, routes); err != nil {
					return fmt.Errorf("repeat narrow sync: %w", err)
				}
				if err := assertRoutes(tt.wantNarrow); err != nil {
					return fmt.Errorf("routes after repeated narrow sync: %w", err)
				}

				if err := syncMigrationConfig(tt.family, tt.wideCIDR, link, routes); err != nil {
					return fmt.Errorf("move route back to wide table: %w", err)
				}
				if err := assertRoutes(tt.wantWide); err != nil {
					return fmt.Errorf("routes after move back to wide table: %w", err)
				}
				if err := assertRouteLinkIndexes(tt.wantWide, linkIndex); err != nil {
					return fmt.Errorf("wide route link indexes: %w", err)
				}
				if err := syncMigrationConfig(tt.family, tt.wideCIDR, link, routes); err != nil {
					return fmt.Errorf("repeat wide sync: %w", err)
				}
				return assertRoutes(tt.wantWide)
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

func syncMigrationConfig(family int, clusterCIDR string, link netlink.Link, routes []*net.IPNet) error {
	clusterNetworks, err := ParseClusterNetworks([]string{clusterCIDR})
	if err != nil {
		return err
	}
	var ipv4, ipv6 net.IP
	if family == netlink.FAMILY_V4 {
		ipv4 = net.ParseIP("10.1.1.1")
	} else {
		ipv6 = net.ParseIP("fd02::1")
	}
	client, err := NewNatClient(ipv4, ipv6, clusterNetworks, constants.EgressBackendIPTables, nil)
	if err != nil {
		return err
	}
	if err := client.SyncInClusterRules(); err != nil {
		return fmt.Errorf("sync in-cluster rules: %w", err)
	}
	if err := client.SyncNat(link, routes, false); err != nil {
		return fmt.Errorf("sync NAT routes: %w", err)
	}
	return nil
}

func assertRouteLinkIndexes(want []routeTuple, expected int) error {
	for _, tuple := range want {
		_, dst, err := net.ParseCIDR(tuple.prefix)
		if err != nil {
			return fmt.Errorf("parse expected prefix %q: %w", tuple.prefix, err)
		}
		routes, err := netlink.RouteListFiltered(tuple.family, &netlink.Route{Table: tuple.table, Dst: dst}, netlink.RT_FILTER_TABLE|netlink.RT_FILTER_DST)
		if err != nil {
			return fmt.Errorf("list family %d table %d prefix %s: %w", tuple.family, tuple.table, tuple.prefix, err)
		}
		if len(routes) != 1 {
			return fmt.Errorf("family %d table %d prefix %s: got %d routes, want 1", tuple.family, tuple.table, tuple.prefix, len(routes))
		}
		if routes[0].LinkIndex != expected {
			return fmt.Errorf("family %d table %d prefix %s: link index got %d, want %d", tuple.family, tuple.table, tuple.prefix, routes[0].LinkIndex, expected)
		}
	}
	return nil
}

type clusterRuleTuple struct {
	family   int
	priority int
	prefix   string
}

func TestNatClient_SyncInClusterRules(t *testing.T) {
	tns := newRouteTestNS(t)
	networks, err := ParseClusterNetworks([]string{"10.20.0.0/16", "10.30.0.0/16", "fd20::/64"})
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, networks)
	if err := tns.Do(func(ns.NetNS) error {
		if err := client.SyncInClusterRules(); err != nil {
			return err
		}
		first := []clusterRuleTuple{
			{netlink.FAMILY_V4, 2000, "10.20.0.0/16"},
			{netlink.FAMILY_V4, 2001, "10.30.0.0/16"},
			{netlink.FAMILY_V6, 2000, "fd20::/64"},
		}
		if err := assertClusterRules(first); err != nil {
			return fmt.Errorf("initial exclusions: %w", err)
		}

		next, err := ParseClusterNetworks([]string{"10.20.0.0/16", "10.40.0.0/16", "fd20::/64", "fd30::/64"})
		if err != nil {
			return err
		}
		client, err = NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), next, constants.EgressBackendIPTables, nil)
		if err != nil {
			return err
		}
		if err := client.SyncInClusterRules(); err != nil {
			return err
		}
		want := []clusterRuleTuple{
			{netlink.FAMILY_V4, 2000, "10.20.0.0/16"}, // retained rules keep their priority
			{netlink.FAMILY_V4, 2001, "10.40.0.0/16"},
			{netlink.FAMILY_V6, 2000, "fd20::/64"},
			{netlink.FAMILY_V6, 2001, "fd30::/64"},
		}
		if err := assertClusterRules(want); err != nil {
			return fmt.Errorf("reconciled exclusions: %w", err)
		}
		before, err := clusterRulesSnapshot()
		if err != nil {
			return err
		}
		if err := client.SyncInClusterRules(); err != nil {
			return err
		}
		after, err := clusterRulesSnapshot()
		if err != nil {
			return err
		}
		if !slices.Equal(before, after) {
			return fmt.Errorf("repeat sync changed exclusions: before %v, after %v", before, after)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNatClient_SyncInClusterRulesDuplicateCIDRs(t *testing.T) {
	tns := newRouteTestNS(t)
	networks, err := ParseClusterNetworks([]string{"10.20.0.0/16", "10.20.0.0/16", "10.20.0.0/16", "10.20.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, networks)
	toDelete := []*netlink.Rule{
		newRule(netlink.FAMILY_V4, mainTableID, 2000),
		newRule(netlink.FAMILY_V6, mainTableID, 2000),
	}
	toDelete[0].Dst = testCIDR(t, "10.20.0.0/16")
	toDelete[1].Dst = testCIDR(t, "fd20::/64")
	if err := tns.Do(func(ns.NetNS) error {
		if err := client.SyncInClusterRules(); err != nil {
			return err
		}
		if err := assertClusterRules([]clusterRuleTuple{
			{netlink.FAMILY_V4, 2000, "10.20.0.0/16"},
			{netlink.FAMILY_V6, 2000, "fc00::/7"},
		}); err != nil {
			return err
		}
		ok, err := client.IsInClusterInitialized()
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("IsInClusterInitialized() = false after duplicate CIDR sync")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNatClient_SyncInClusterRulesFamilyOverrides(t *testing.T) {
	tests := []struct {
		name  string
		cidrs []string
		want  []clusterRuleTuple
	}{
		{"IPv4 only override preserves IPv6 defaults", []string{"10.20.0.0/16"}, []clusterRuleTuple{
			{netlink.FAMILY_V4, 2000, "10.20.0.0/16"},
			{netlink.FAMILY_V6, 2000, "fc00::/7"},
		}},
		{"IPv6 only override preserves IPv4 defaults", []string{"fd20::/64"}, []clusterRuleTuple{
			{netlink.FAMILY_V4, 2000, "10.0.0.0/8"},
			{netlink.FAMILY_V4, 2001, "172.16.0.0/12"},
			{netlink.FAMILY_V4, 2002, "192.168.0.0/16"},
			{netlink.FAMILY_V6, 2000, "fd20::/64"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tns := newRouteTestNS(t)
			networks, err := ParseClusterNetworks(tt.cidrs)
			if err != nil {
				t.Fatal(err)
			}
			client := newTestClient(t, networks)
			if err := tns.Do(func(ns.NetNS) error {
				if err := client.SyncInClusterRules(); err != nil {
					return err
				}
				return assertClusterRules(tt.want)
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNatClient_SyncInClusterRulesDefaultExclusions(t *testing.T) {
	tests := []struct {
		name   string
		family int
		prefix string
	}{
		{"IPv4", netlink.FAMILY_V4, "0.0.0.0/0"},
		{"IPv6", netlink.FAMILY_V6, "::/0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tns := newRouteTestNS(t)
			networks, err := ParseClusterNetworks([]string{tt.prefix})
			if err != nil {
				t.Fatal(err)
			}
			client := newTestClient(t, networks)
			if err := tns.Do(func(ns.NetNS) error {
				if err := client.SyncInClusterRules(); err != nil {
					return err
				}
				wantInitial := []clusterRuleTuple{{tt.family, 2000, tt.prefix}}
				if tt.family == netlink.FAMILY_V4 {
					wantInitial = append(wantInitial, clusterRuleTuple{netlink.FAMILY_V6, 2000, "fc00::/7"})
				} else {
					wantInitial = append(wantInitial,
						clusterRuleTuple{netlink.FAMILY_V4, 2000, "10.0.0.0/8"},
						clusterRuleTuple{netlink.FAMILY_V4, 2001, "172.16.0.0/12"},
						clusterRuleTuple{netlink.FAMILY_V4, 2002, "192.168.0.0/16"},
					)
				}
				if err := assertClusterRules(wantInitial); err != nil {
					return err
				}
				client, err = NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), nil, constants.EgressBackendIPTables, nil)
				if err != nil {
					return err
				}
				if err := client.SyncInClusterRules(); err != nil {
					return err
				}
				return assertClusterRules([]clusterRuleTuple{
					{netlink.FAMILY_V4, 2000, "10.0.0.0/8"},
					{netlink.FAMILY_V4, 2001, "172.16.0.0/12"},
					{netlink.FAMILY_V4, 2002, "192.168.0.0/16"},
					{netlink.FAMILY_V6, 2000, "fc00::/7"},
				})
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNatClient_IsInClusterInitialized(t *testing.T) {
	tns := newRouteTestNS(t)
	networks, err := ParseClusterNetworks([]string{"10.20.0.0/16", "fd20::/64"})
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, networks)
	toDelete := []*netlink.Rule{
		newRule(netlink.FAMILY_V4, mainTableID, 2000),
		newRule(netlink.FAMILY_V6, mainTableID, 2000),
	}
	toDelete[0].Dst = testCIDR(t, "10.20.0.0/16")
	toDelete[1].Dst = testCIDR(t, "fd20::/64")
	if err := tns.Do(func(ns.NetNS) error {
		check := func(want bool) error {
			got, err := client.IsInClusterInitialized()
			if got != want || (want && err != nil) {
				return fmt.Errorf("IsInClusterInitialized() = (%v, %v), want bool %v and no error", got, err, want)
			}
			return nil
		}
		if err := check(false); err != nil {
			return fmt.Errorf("before sync: %w", err)
		}
		if err := client.SyncInClusterRules(); err != nil {
			return err
		}
		if err := check(true); err != nil {
			return fmt.Errorf("after sync: %w", err)
		}
		for i, required := range []string{"10.20.0.0/16", "fd20::/64"} {
			rule := toDelete[i]
			if err := netlink.RuleDel(rule); err != nil {
				return fmt.Errorf("delete required rule %s: %w", required, err)
			}
			if err := check(false); err != nil {
				return fmt.Errorf("after deleting %s: %w", required, err)
			}
			if err := client.SyncInClusterRules(); err != nil {
				return err
			}
			if err := check(true); err != nil {
				return fmt.Errorf("after restoring %s: %w", required, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func newTestClient(t *testing.T, networks *ClusterNetworks) *NatClient {
	t.Helper()
	client, err := NewNatClient(net.ParseIP("10.1.1.1"), net.ParseIP("fd02::1"), networks, constants.EgressBackendIPTables, nil)
	if err != nil {
		t.Fatalf("NewNatClient: %v", err)
	}
	return client
}

func assertClusterRules(want []clusterRuleTuple) error {
	got, err := clusterRulesSnapshot()
	if err != nil {
		return err
	}
	want = append([]clusterRuleTuple(nil), want...)
	slices.SortFunc(want, compareClusterRuleTuple)
	if !slices.Equal(got, want) {
		return fmt.Errorf("reserved main-table rules got %v, want %v", got, want)
	}
	return nil
}

func clusterRulesSnapshot() ([]clusterRuleTuple, error) {
	var got []clusterRuleTuple
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		rules, err := netlink.RuleListFiltered(family, &netlink.Rule{Table: mainTableID}, netlink.RT_FILTER_TABLE)
		if err != nil {
			return nil, fmt.Errorf("list family %d main-table rules: %w", family, err)
		}
		for _, rule := range rules {
			if rule.Priority < 2000 || rule.Priority > 2099 {
				continue
			}
			dst := rule.Dst
			if dst == nil {
				dst = getDefaultGlobalDst(family)
			}
			got = append(got, clusterRuleTuple{family, rule.Priority, dst.String()})
		}
	}
	slices.SortFunc(got, compareClusterRuleTuple)
	return got, nil
}

func compareClusterRuleTuple(a, b clusterRuleTuple) int {
	if a.family != b.family {
		return a.family - b.family
	}
	if a.priority != b.priority {
		return a.priority - b.priority
	}
	if a.prefix < b.prefix {
		return -1
	}
	if a.prefix > b.prefix {
		return 1
	}
	return 0
}

// TestNatClient_ReconcileRoutes exercises route deletion across both policy
// routing tables and both IP families.  Keep this test serial: nftables has
// package-global state and the privileged netfilter tests share it.
func TestNatClient_ReconcileRoutes(t *testing.T) {
	tests := []struct {
		name       string
		removeLink string
		remaining  []routeTuple
	}{
		{
			name:       "remove IPv6 routes without disturbing IPv4 on another link",
			removeLink: "route-v6",
			remaining: []routeTuple{
				{netlink.FAMILY_V4, ncNarrowTableID, "10.2.0.0/16", "route-v4"},
				{netlink.FAMILY_V4, ncWideTableID, "0.0.0.0/0", "route-v4"},
			},
		},
		{
			name:       "remove IPv4 routes without disturbing IPv6 on another link",
			removeLink: "route-v4",
			remaining: []routeTuple{
				{netlink.FAMILY_V6, ncNarrowTableID, "fd02::/64", "route-v6"},
				{netlink.FAMILY_V6, ncWideTableID, "::/0", "route-v6"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tns := newRouteTestNS(t)
			client := newTestClient(t, nil)
			v4Routes := []*net.IPNet{testCIDR(t, "10.2.0.0/16"), testCIDR(t, "0.0.0.0/0")}
			v6Routes := []*net.IPNet{testCIDR(t, "fd02::/64"), testCIDR(t, "::/0")}
			if err := tns.Do(func(ns.NetNS) error {
				v4Link, err := newDummyLink("route-v4")
				if err != nil {
					return err
				}
				v6Link, err := newDummyLink("route-v6")
				if err != nil {
					return err
				}
				if err := client.Init(); err != nil {
					return fmt.Errorf("initialize NatClient: %w", err)
				}

				if err := client.SyncNat(v4Link, v4Routes, false); err != nil {
					return fmt.Errorf("initial SyncNat on v4 link: %w", err)
				}
				if err := client.SyncNat(v6Link, v6Routes, false); err != nil {
					return fmt.Errorf("initial SyncNat on v6 link: %w", err)
				}

				if err := assertRoutes([]routeTuple{
					{netlink.FAMILY_V4, ncNarrowTableID, "10.2.0.0/16", "route-v4"},
					{netlink.FAMILY_V4, ncWideTableID, "0.0.0.0/0", "route-v4"},
					{netlink.FAMILY_V6, ncNarrowTableID, "fd02::/64", "route-v6"},
					{netlink.FAMILY_V6, ncWideTableID, "::/0", "route-v6"},
				}); err != nil {
					return fmt.Errorf("initial route state: %w", err)
				}

				removeLink := v4Link
				if tt.removeLink == "route-v6" {
					removeLink = v6Link
				}
				// A nil slice and a non-nil empty slice are both supported cleanup inputs.
				if err := client.SyncNat(removeLink, nil, false); err != nil {
					return fmt.Errorf("SyncNat cleanup with nil destinations: %w", err)
				}
				if err := assertRoutes(tt.remaining); err != nil {
					return fmt.Errorf("routes after nil cleanup: %w", err)
				}

				if err := client.SyncNat(v4Link, v4Routes, false); err != nil {
					return fmt.Errorf("initial SyncNat on v4 link: %w", err)
				}
				if err := client.SyncNat(v6Link, v6Routes, false); err != nil {
					return fmt.Errorf("initial SyncNat on v6 link: %w", err)
				}

				if err := assertRoutes([]routeTuple{
					{netlink.FAMILY_V4, ncNarrowTableID, "10.2.0.0/16", "route-v4"},
					{netlink.FAMILY_V4, ncWideTableID, "0.0.0.0/0", "route-v4"},
					{netlink.FAMILY_V6, ncNarrowTableID, "fd02::/64", "route-v6"},
					{netlink.FAMILY_V6, ncWideTableID, "::/0", "route-v6"},
				}); err != nil {
					return fmt.Errorf("initial route state: %w", err)
				}

				if err := client.SyncNat(removeLink, []*net.IPNet{}, false); err != nil {
					return fmt.Errorf("repeated SyncNat cleanup with empty destinations: %w", err)
				}
				return assertRoutes(tt.remaining)
			}); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestNatClient_ReconcileRoutesTracksDesiredFamilies verifies that one
// dual-stack client's route inventory follows the desired subnet families.
func TestNatClient_ReconcileRoutesTracksDesiredFamilies(t *testing.T) {
	tns := newRouteTestNS(t)
	client := newTestClient(t, nil)
	v4 := testCIDR(t, "10.2.0.0/16")
	v6 := testCIDR(t, "fd02::/64")
	if err := tns.Do(func(ns.NetNS) error {
		link, err := newDummyLink("route-family")
		if err != nil {
			return err
		}
		if err := client.Init(); err != nil {
			return fmt.Errorf("initialize NatClient: %w", err)
		}

		if err := client.SyncNat(link, []*net.IPNet{v4, v6}, false); err != nil {
			return fmt.Errorf("initial dual-stack SyncNat: %w", err)
		}
		if err := assertRoutes([]routeTuple{
			{netlink.FAMILY_V4, ncNarrowTableID, "10.2.0.0/16", "route-family"},
			{netlink.FAMILY_V6, ncNarrowTableID, "fd02::/64", "route-family"},
		}); err != nil {
			return fmt.Errorf("initial dual-stack routes: %w", err)
		}

		if err := client.SyncNat(link, []*net.IPNet{v4}, false); err != nil {
			return fmt.Errorf("sync IPv4-only destinations: %w", err)
		}
		if err := assertRoutes([]routeTuple{
			{netlink.FAMILY_V4, ncNarrowTableID, "10.2.0.0/16", "route-family"},
		}); err != nil {
			return fmt.Errorf("routes after dual-stack to IPv4-only destinations: %w", err)
		}

		if err := client.SyncNat(link, []*net.IPNet{v6}, false); err != nil {
			return fmt.Errorf("sync IPv6-only destinations: %w", err)
		}
		return assertRoutes([]routeTuple{
			{netlink.FAMILY_V6, ncNarrowTableID, "fd02::/64", "route-family"},
		})
	}); err != nil {
		t.Fatal(err)
	}
}

type routeTuple struct {
	family int
	table  int
	prefix string
	link   string
}

func newRouteTestNS(t *testing.T) ns.NetNS {
	t.Helper()
	tns, err := testutils.NewNS()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tns.Close(); err != nil {
			t.Errorf("close test netns: %v", err)
		}
		if err := testutils.UnmountNS(tns); err != nil {
			t.Errorf("unmount test netns: %v", err)
		}
	})
	return tns
}

func newDummyLink(name string) (netlink.Link, error) {
	attrs := netlink.NewLinkAttrs()
	attrs.Name = name
	dummy := &netlink.Dummy{LinkAttrs: attrs}
	if err := netlink.LinkAdd(dummy); err != nil {
		return nil, fmt.Errorf("add dummy %q: %w", name, err)
	}
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("lookup dummy %q: %w", name, err)
	}
	return link, nil
}

func testCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("parse CIDR %q: %v", cidr, err)
	}
	return ipNet
}

func assertRoutes(want []routeTuple) error {
	var got []routeTuple
	for _, family := range []int{netlink.FAMILY_V4, netlink.FAMILY_V6} {
		for _, table := range []int{ncNarrowTableID, ncWideTableID} {
			routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: table}, netlink.RT_FILTER_TABLE)
			if err != nil {
				return fmt.Errorf("list family %d table %d routes: %w", family, table, err)
			}
			for _, route := range routes {
				link, err := netlink.LinkByIndex(route.LinkIndex)
				if err != nil {
					return fmt.Errorf("lookup route link index %d: %w", route.LinkIndex, err)
				}
				got = append(got, routeTuple{family, table, route.Dst.String(), link.Attrs().Name})
			}
		}
	}
	slices.SortFunc(got, compareRouteTuple)
	want = append([]routeTuple(nil), want...)
	slices.SortFunc(want, compareRouteTuple)
	if !slices.Equal(got, want) {
		return fmt.Errorf("routes got %v, want %v", got, want)
	}
	return nil
}

func compareRouteTuple(a, b routeTuple) int {
	if a.family != b.family {
		return a.family - b.family
	}
	if a.table != b.table {
		return a.table - b.table
	}
	if a.prefix != b.prefix {
		if a.prefix < b.prefix {
			return -1
		}
		return 1
	}
	if a.link < b.link {
		return -1
	}
	if a.link > b.link {
		return 1
	}
	return 0
}

func checkInitRules(nc *NatClient) error {
	if nc.ipv4 != nil {
		if err := checkInitRulesForFamily(netlink.FAMILY_V4, nc.inCluster.v4); err != nil {
			return fmt.Errorf("IPv4: %w", err)
		}
	}
	if nc.ipv6 != nil {
		if err := checkInitRulesForFamily(netlink.FAMILY_V6, nc.inCluster.v6); err != nil {
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
	if err := checkNatClientRoutesByFamily(nc.ipv4, nc.inCluster.v4, v4Nets, netlink.FAMILY_V4); err != nil {
		return fmt.Errorf("IPv4: %w", err)
	}
	if err := checkNatClientRoutesByFamily(nc.ipv6, nc.inCluster.v6, v6Nets, netlink.FAMILY_V6); err != nil {
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
