package founat

import (
	"fmt"
	"net"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
)

const (
	egressTableID    = 118
	egressProtocolID = 30
	egressRulePrio   = 2000

	egressDummy = "egress-dummy"
)

// Egress represents NAT and routing service running on egress Pods.
// Methods are idempotent; i.e. they can be called multiple times.
type Egress interface {
	Init() error
	AddClient(net.IP, netlink.Link) error
}

// NewEgress creates an Egress
func NewEgress(iface string, ipv4, ipv6 net.IP, useNFT bool) Egress {
	if ipv4 != nil && ipv4.To4() == nil {
		panic("invalid IPv4 address")
	}
	if ipv6 != nil && ipv6.To4() != nil {
		panic("invalid IPv6 address")
	}
	return &egress{
		iface:  iface,
		ipv4:   ipv4,
		ipv6:   ipv6,
		useNFT: useNFT,
	}
}

type egress struct {
	iface  string
	ipv4   net.IP
	ipv6   net.IP
	useNFT bool

	mu sync.Mutex
}

func (e *egress) newRule(family int) *netlink.Rule {
	r := netlink.NewRule()
	r.Family = family
	r.IifName = e.iface
	r.Table = egressTableID
	r.Priority = egressRulePrio
	return r
}

func (e *egress) Init() error {
	// avoid double initialization in case the program restarts
	_, err := netlink.LinkByName(egressDummy)
	if err == nil {
		return nil
	}
	if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return err
	}

	if e.useNFT {
		conn, err := nftables.New()
		if err != nil {
			return fmt.Errorf("failed to create nftables connection: %w", err)
		}

		if e.ipv4 != nil {
			ipNet := netlink.NewIPNet(e.ipv4)
			_, ipNetParsed, err := net.ParseCIDR(ipNet.String())
			if err != nil {
				return fmt.Errorf("failed to parse IPv4 network %s: %w", ipNet.String(), err)
			}

			natTable := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: "nat"}
			conn.AddTable(natTable)

			postRoutingChain := &nftables.Chain{
				Name:     "POSTROUTING",
				Table:    natTable,
				Type:     nftables.ChainTypeNAT,
				Hooknum:  nftables.ChainHookPostrouting,
				Priority: nftables.ChainPriorityNATSource,
			}
			conn.AddChain(postRoutingChain)

			masqRule := &nftables.Rule{
				Table: natTable,
				Chain: postRoutingChain,
				Exprs: []expr.Any{
					&expr.Payload{
						DestRegister: 1,
						Base:         expr.PayloadBaseNetworkHeader,
						Offset:       12,
						Len:          4,
					},
					&expr.Bitwise{
						SourceRegister: 1,
						DestRegister:   1,
						Len:            4,
						Mask:           ipNetParsed.Mask,
						Xor:            binaryutil.NativeEndian.PutUint32(0),
					},
					&expr.Cmp{
						Op:       expr.CmpOpNeq,
						Register: 1,
						Data:     ipNetParsed.IP.To4(),
					},
					&expr.Meta{
						Key:      expr.MetaKeyOIFNAME,
						Register: 1,
					},
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     []byte(e.iface + "\x00"),
					},
					&expr.Counter{},
					&expr.Masq{},
				},
			}
			conn.AddRule(masqRule)

			filterTable := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: "filter"}
			conn.AddTable(filterTable)

			forwardChain := &nftables.Chain{
				Name:     "FORWARD",
				Table:    filterTable,
				Type:     nftables.ChainTypeFilter,
				Hooknum:  nftables.ChainHookForward,
				Priority: nftables.ChainPriorityFilter,
			}
			conn.AddChain(forwardChain)

			dropRule := &nftables.Rule{
				Table: filterTable,
				Chain: forwardChain,
				Exprs: []expr.Any{
					&expr.Meta{
						Key:      expr.MetaKeyOIFNAME,
						Register: 1,
					},
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     []byte(e.iface + "\x00"),
					},
					&expr.Ct{
						Register: 1,
						Key:      expr.CtKeySTATE,
					},
					&expr.Bitwise{
						SourceRegister: 1,
						DestRegister:   1,
						Len:            4,
						Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitINVALID),
						Xor:            binaryutil.NativeEndian.PutUint32(0),
					},
					&expr.Cmp{
						Op:       expr.CmpOpNeq,
						Register: 1,
						Data:     binaryutil.NativeEndian.PutUint32(0),
					},
					&expr.Counter{},
					&expr.Verdict{
						Kind: expr.VerdictDrop,
					},
				},
			}
			conn.AddRule(dropRule)

			rule := e.newRule(netlink.FAMILY_V4)
			if err := netlink.RuleAdd(rule); err != nil {
				return fmt.Errorf("netlink: failed to add egress rule for IPv4: %w", err)
			}
		}

		if e.ipv6 != nil {
			ipNet := netlink.NewIPNet(e.ipv6)
			_, ipNetParsed, err := net.ParseCIDR(ipNet.String())
			if err != nil {
				return fmt.Errorf("failed to parse IPv6 network %s: %w", ipNet.String(), err)
			}

			natTable := &nftables.Table{Family: nftables.TableFamilyIPv6, Name: "nat"}
			conn.AddTable(natTable)

			postRoutingChain := &nftables.Chain{
				Name:     "POSTROUTING",
				Table:    natTable,
				Type:     nftables.ChainTypeNAT,
				Hooknum:  nftables.ChainHookPostrouting,
				Priority: nftables.ChainPriorityNATSource,
			}
			conn.AddChain(postRoutingChain)

			masqRule := &nftables.Rule{
				Table: natTable,
				Chain: postRoutingChain,
				Exprs: []expr.Any{
					&expr.Payload{
						DestRegister: 1,
						Base:         expr.PayloadBaseNetworkHeader,
						Offset:       8,
						Len:          16,
					},
					&expr.Bitwise{
						SourceRegister: 1,
						DestRegister:   1,
						Len:            16,
						Mask:           ipNetParsed.Mask,
						Xor:            make([]byte, 16),
					},
					&expr.Cmp{
						Op:       expr.CmpOpNeq,
						Register: 1,
						Data:     ipNetParsed.IP.To16(),
					},
					&expr.Meta{
						Key:      expr.MetaKeyOIFNAME,
						Register: 1,
					},
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     []byte(e.iface + "\x00"),
					},
					&expr.Counter{},
					&expr.Masq{},
				},
			}
			conn.AddRule(masqRule)

			filterTable := &nftables.Table{Family: nftables.TableFamilyIPv6, Name: "filter"}
			conn.AddTable(filterTable)

			forwardChain := &nftables.Chain{
				Name:     "FORWARD",
				Table:    filterTable,
				Type:     nftables.ChainTypeFilter,
				Hooknum:  nftables.ChainHookForward,
				Priority: nftables.ChainPriorityFilter,
			}
			conn.AddChain(forwardChain)

			dropRule := &nftables.Rule{
				Table: filterTable,
				Chain: forwardChain,
				Exprs: []expr.Any{
					&expr.Meta{
						Key:      expr.MetaKeyOIFNAME,
						Register: 1,
					},
					&expr.Cmp{
						Op:       expr.CmpOpEq,
						Register: 1,
						Data:     []byte(e.iface + "\x00"),
					},
					&expr.Ct{
						Register: 1,
						Key:      expr.CtKeySTATE,
					},
					&expr.Bitwise{
						SourceRegister: 1,
						DestRegister:   1,
						Len:            4,
						Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitINVALID),
						Xor:            binaryutil.NativeEndian.PutUint32(0),
					},
					&expr.Cmp{
						Op:       expr.CmpOpNeq,
						Register: 1,
						Data:     binaryutil.NativeEndian.PutUint32(0),
					},
					&expr.Counter{},
					&expr.Verdict{
						Kind: expr.VerdictDrop,
					},
				},
			}
			conn.AddRule(dropRule)

			rule := e.newRule(netlink.FAMILY_V6)
			if err := netlink.RuleAdd(rule); err != nil {
				return fmt.Errorf("netlink: failed to add egress rule for IPv6: %w", err)
			}
		}

		if err := conn.Flush(); err != nil {
			return fmt.Errorf("failed to flush nftables rules: %w", err)
		}
	} else {
		if e.ipv4 != nil {
			ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
			if err != nil {
				return err
			}
			ipn := netlink.NewIPNet(e.ipv4)
			err = ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", e.iface, "-j", "MASQUERADE")
			if err != nil {
				return fmt.Errorf("failed to setup masquerade rule for IPv4: %w", err)
			}
			if err := ipt.Append("filter", "FORWARD", "-o", e.iface, "-m", "state", "--state", "INVALID", "-j", "DROP"); err != nil {
				return fmt.Errorf("failed to setup drop rule for invalid packets: %w", err)
			}

			rule := e.newRule(netlink.FAMILY_V4)
			if err := netlink.RuleAdd(rule); err != nil {
				return fmt.Errorf("netlink: failed to add egress rule for IPv4: %w", err)
			}
		}
		if e.ipv6 != nil {
			ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv6)
			if err != nil {
				return err
			}
			ipn := netlink.NewIPNet(e.ipv6)
			err = ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", e.iface, "-j", "MASQUERADE")
			if err != nil {
				return fmt.Errorf("failed to setup masquerade rule for IPv6: %w", err)
			}
			if err := ipt.Append("filter", "FORWARD", "-o", e.iface, "-m", "state", "--state", "INVALID", "-j", "DROP"); err != nil {
				return fmt.Errorf("failed to setup drop rule for invalid packets: %w", err)
			}

			rule := e.newRule(netlink.FAMILY_V6)
			if err := netlink.RuleAdd(rule); err != nil {
				return fmt.Errorf("netlink: failed to add egress rule for IPv6: %w", err)
			}
		}
	}

	attrs := netlink.NewLinkAttrs()
	attrs.Name = egressDummy
	return netlink.LinkAdd(&netlink.Dummy{LinkAttrs: attrs})
}

func (e *egress) AddClient(addr net.IP, link netlink.Link) error {
	// Note:
	// The following checks are not necessary in fact because,
	// prior to this point, the support for the IP family is tested
	// by FouTunnel.AddPeer().  If the test fails, then no `link`
	// is created and this method will not be called.
	// Just as a safeguard.
	if addr.To4() != nil && e.ipv4 == nil {
		return ErrIPFamilyMismatch
	}
	if addr.To4() == nil && e.ipv6 == nil {
		return ErrIPFamilyMismatch
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	family := netlink.FAMILY_V4
	if addr.To4() == nil {
		family = netlink.FAMILY_V6
	}
	routes, err := netlink.RouteListFiltered(family, &netlink.Route{Table: egressTableID}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return fmt.Errorf("netlink: failed to list routes in table %d: %w", egressTableID, err)
	}

	for _, r := range routes {
		if r.Dst == nil {
			continue
		}
		if r.Dst.IP.Equal(addr) {
			return nil
		}
	}

	// link up here to minimize the down time
	// See https://github.com/cybozu-go/coil/issues/287.
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("netlink: failed to link up %s: %w", link.Attrs().Name, err)
	}
	err = netlink.RouteAdd(&netlink.Route{
		Dst:       netlink.NewIPNet(addr),
		LinkIndex: link.Attrs().Index,
		Table:     egressTableID,
		Protocol:  egressProtocolID,
	})
	if err != nil {
		return fmt.Errorf("netlink: failed to add %s to table %d: %w", addr.String(), egressTableID, err)
	}
	return nil
}
