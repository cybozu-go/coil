package netfilter

import (
	"fmt"
	"net"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/vishvananda/netlink"
)

const (
	nftRegister = 1

	ipv4SrcOffset = 12
	ipv4SrcLen    = 4
	ipv6SrcOffset = 8
	ipv6SrcLen    = 16
)

func setNFTablesMasqRules(family nftables.TableFamily, iface string, ip net.IP) (err error) {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("failed to create nftables connection: %w", err)
	}

	ipn := netlink.NewIPNet(ip)
	_, ipnParsed, err := net.ParseCIDR(ipn.String())
	if err != nil {
		return fmt.Errorf("failed to parse network %s: %w", ipn.String(), err)
	}

	t := &nftables.Table{Family: family, Name: natTable}
	conn.AddTable(t)

	c := &nftables.Chain{
		Name:     natChain,
		Table:    t,
		Type:     nftables.ChainTypeNAT,
		Hooknum:  nftables.ChainHookPostrouting,
		Priority: nftables.ChainPriorityNATSource,
	}
	conn.AddChain(c)

	var offset, length uint32
	var xor, ipData []byte
	switch family {
	case nftables.TableFamilyIPv4:
		offset = ipv4SrcOffset
		length = ipv4SrcLen
		xor = binaryutil.NativeEndian.PutUint32(0)
		ipData = ipnParsed.IP.To4()
	case nftables.TableFamilyIPv6:
		offset = ipv6SrcOffset
		length = ipv6SrcLen
		xor = make([]byte, ipv6SrcLen)
		ipData = ipnParsed.IP.To16()
	default:
		return fmt.Errorf("invalid table family %d", family)
	}

	// ex. nft add rule ip nat POSTROUTING ip saddr != 10.0.0.0/24 oifname "eth0" counter masquerade
	masqRule := &nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
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
				Mask:           ipnParsed.Mask,
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
				Data:     []byte(iface + "\x00"),
			},
			&expr.Counter{},
			&expr.Masq{},
		},
	}
	conn.AddRule(masqRule)

	t = &nftables.Table{Family: family, Name: filterTable}
	conn.AddTable(t)

	// ex. nft add chain ip filter FORWARD { type filter hook forward priority filter \; }
	c = &nftables.Chain{
		Name:     filterChain,
		Table:    t,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookForward,
		Priority: nftables.ChainPriorityFilter,
	}
	conn.AddChain(c)

	// Drop invalid or malformed packets from passing through the network.
	// ex. nft add rule ip filter FORWARD oifname "eth0" ct state invalid counter drop
	dropRule := &nftables.Rule{
		Table: t,
		Chain: c,
		Exprs: []expr.Any{
			&expr.Meta{
				Key:      expr.MetaKeyOIFNAME,
				Register: nftRegister,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: nftRegister,
				Data:     []byte(iface + "\x00"),
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

	if err := conn.Flush(); err != nil {
		return fmt.Errorf("failed to flush nftables rules: %w", err)
	}
	return nil
}
