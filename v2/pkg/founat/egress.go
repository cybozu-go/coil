package founat

import (
	"fmt"
	"net"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/cybozu-go/coil/v2/pkg/constants"
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

	// nftables payload offsets and lengths
	ipv4SrcOffset = 12
	ipv4SrcLen    = 4
	ipv6SrcOffset = 8
	ipv6SrcLen    = 16

	// nftables register number
	nftRegister = 1
)

// Egress represents NAT and routing service running on egress Pods.
// Methods are idempotent; i.e. they can be called multiple times.
type Egress interface {
	Init() error
	AddClient(net.IP, netlink.Link) error
}

// NewEgress creates an Egress
func NewEgress(iface string, ipv4, ipv6 net.IP, backend string) Egress {
	if ipv4 != nil && ipv4.To4() == nil {
		panic("invalid IPv4 address")
	}
	if ipv6 != nil && ipv6.To4() != nil {
		panic("invalid IPv6 address")
	}
	return &egress{
		iface:   iface,
		ipv4:    ipv4,
		ipv6:    ipv6,
		backend: backend,
	}
}

type egress struct {
	iface   string
	ipv4    net.IP
	ipv6    net.IP
	backend string

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

func (e *egress) addEgressRule(family int) error {
	rule := e.newRule(family)
	if err := netlink.RuleAdd(rule); err != nil {
		ipVersion := "IPv4"
		if family == netlink.FAMILY_V6 {
			ipVersion = "IPv6"
		}
		return fmt.Errorf("netlink: failed to add egress rule for %s: %w", ipVersion, err)
	}
	return nil
}

func (e *egress) addNFTablesRules(conn *nftables.Conn, family nftables.TableFamily, ip net.IP) error {
	ipNet := netlink.NewIPNet(ip)
	_, ipNetParsed, err := net.ParseCIDR(ipNet.String())
	if err != nil {
		return fmt.Errorf("failed to parse network %s: %w", ipNet.String(), err)
	}

	natTable := &nftables.Table{Family: family, Name: "nat"}
	conn.AddTable(natTable)

	postRoutingChain := &nftables.Chain{
		Name:     "POSTROUTING",
		Table:    natTable,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPostrouting,
		Priority: nftables.ChainPriorityNATSource,
	}
	conn.AddChain(postRoutingChain)

	var offset, length uint32
	var xor, ipData []byte
	if family == nftables.TableFamilyIPv6 {
		offset = ipv6SrcOffset
		length = ipv6SrcLen
		xor = make([]byte, ipv6SrcLen)
		ipData = ipNetParsed.IP.To16()
	} else {
		offset = ipv4SrcOffset
		length = ipv4SrcLen
		xor = binaryutil.NativeEndian.PutUint32(0)
		ipData = ipNetParsed.IP.To4()
	}

	// ex. nft add rule ip nat POSTROUTING ip saddr != 10.0.0.0/24 oifname "eth0" counter masquerade
	masqExprs := []expr.Any{
		&expr.Payload{
			DestRegister: nftRegister,
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       offset,
			Len:          length,
		},
		&expr.Bitwise{
			SourceRegister: nftRegister,
			DestRegister:   nftRegister,
			Len:            length,
			Mask:           ipNetParsed.Mask,
			Xor:            xor,
		},
		&expr.Cmp{
			Op:       expr.CmpOpNeq,
			Register: nftRegister,
			Data:     ipData,
		},
		&expr.Meta{
			Key:      expr.MetaKeyOIFNAME,
			Register: nftRegister,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: nftRegister,
			Data:     []byte(e.iface + "\x00"),
		},
		&expr.Counter{},
		&expr.Masq{},
	}

	masqRule := &nftables.Rule{
		Table: natTable,
		Chain: postRoutingChain,
		Exprs: masqExprs,
	}
	conn.AddRule(masqRule)

	filterTable := &nftables.Table{Family: family, Name: "filter"}
	conn.AddTable(filterTable)

	// ex. nft add chain ip filter FORWARD { type filter hook forward priority filter \; }
	forwardChain := &nftables.Chain{
		Name:     "FORWARD",
		Table:    filterTable,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityFilter,
	}
	conn.AddChain(forwardChain)

	// Drop invalid or malformed packets from passing through the network.
	// ex. nft add rule ip filter FORWARD oifname "eth0" ct state invalid counter drop
	dropRule := &nftables.Rule{
		Table: filterTable,
		Chain: forwardChain,
		Exprs: []expr.Any{
			&expr.Meta{
				Key:      expr.MetaKeyOIFNAME,
				Register: nftRegister,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: nftRegister,
				Data:     []byte(e.iface + "\x00"),
			},
			&expr.Ct{
				Register: nftRegister,
				Key:      expr.CtKeySTATE,
			},
			&expr.Bitwise{
				SourceRegister: nftRegister,
				DestRegister:   nftRegister,
				Len:            ipv4SrcLen,
				Mask:           binaryutil.NativeEndian.PutUint32(expr.CtStateBitINVALID),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: nftRegister,
				Data:     binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Counter{},
			&expr.Verdict{
				Kind: expr.VerdictDrop,
			},
		},
	}
	conn.AddRule(dropRule)

	return nil
}

func (e *egress) addIPTablesRules(protocol iptables.Protocol, ip net.IP) error {
	ipt, err := iptables.NewWithProtocol(protocol)
	if err != nil {
		return err
	}
	ipn := netlink.NewIPNet(ip)
	if err := ipt.Append("nat", "POSTROUTING", "!", "-s", ipn.String(), "-o", e.iface, "-j", "MASQUERADE"); err != nil {
		ipVersion := "IPv4"
		if protocol == iptables.ProtocolIPv6 {
			ipVersion = "IPv6"
		}
		return fmt.Errorf("failed to setup masquerade rule for %s: %w", ipVersion, err)
	}
	if err := ipt.Append("filter", "FORWARD", "-o", e.iface, "-m", "state", "--state", "INVALID", "-j", "DROP"); err != nil {
		return fmt.Errorf("failed to setup drop rule for invalid packets: %w", err)
	}
	return nil
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

	if e.backend == constants.BackendNFTables {
		conn, err := nftables.New()
		if err != nil {
			return fmt.Errorf("failed to create nftables connection: %w", err)
		}

		if e.ipv4 != nil {
			if err := e.addNFTablesRules(conn, nftables.TableFamilyIPv4, e.ipv4); err != nil {
				return err
			}
		}

		if e.ipv6 != nil {
			if err := e.addNFTablesRules(conn, nftables.TableFamilyIPv6, e.ipv6); err != nil {
				return err
			}
		}

		if err := conn.Flush(); err != nil {
			return fmt.Errorf("failed to flush nftables rules: %w", err)
		}
	} else {
		if e.ipv4 != nil {
			if err := e.addIPTablesRules(iptables.ProtocolIPv4, e.ipv4); err != nil {
				return err
			}
		}
		if e.ipv6 != nil {
			if err := e.addIPTablesRules(iptables.ProtocolIPv6, e.ipv6); err != nil {
				return err
			}
		}
	}

	if e.ipv4 != nil {
		if err := e.addEgressRule(netlink.FAMILY_V4); err != nil {
			return err
		}
	}
	if e.ipv6 != nil {
		if err := e.addEgressRule(netlink.FAMILY_V6); err != nil {
			return err
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
