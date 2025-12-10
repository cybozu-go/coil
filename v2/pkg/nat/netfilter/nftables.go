package netfilter

import (
	"bytes"
	"fmt"
	"net"
	"strings"

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

	// Rule identifiers for deduplication
	nftRuleIDInputPrefix  = "coil-input-"
	nftRuleIDOutputPrefix = "coil-output-"
)

func setNFTablesMasqRules(family int, iface string, ip net.IP) (err error) {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("failed to create nftables connection: %w", err)
	}

	ipn := netlink.NewIPNet(ip)
	_, ipnParsed, err := net.ParseCIDR(ipn.String())
	if err != nil {
		return fmt.Errorf("failed to parse network %s: %w", ipn.String(), err)
	}

	nf, err := netlinkToNFTablesFamily(family)
	if err != nil {
		return err
	}
	t := &nftables.Table{Family: nf, Name: natTable}
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
	switch nf {
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

	t = &nftables.Table{Family: nf, Name: filterTable}
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

func setNFTablesConnmarkRules(family int, link netlink.Link) error {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("failed to create nftables connection: %w", err)
	}

	nf, err := netlinkToNFTablesFamily(family)
	if err != nil {
		return err
	}

	// AddTable is idempotent - it will create the table if it doesn't exist,
	// or return the existing table if it does.
	mangle := &nftables.Table{Family: nf, Name: mangleTable}
	mangle = conn.AddTable(mangle)

	if err := setNFTablesInputConnmarkRules(conn, mangle, link); err != nil {
		return fmt.Errorf("failed to configure NFT input rules: %w", err)
	}

	if err := setNFTablesOutputConnmarkRules(conn, mangle, link); err != nil {
		return fmt.Errorf("failed to configure NFT input rules: %w", err)
	}

	if err := conn.Flush(); err != nil {
		return fmt.Errorf("failed to flush nftables rules: %w", err)
	}

	return nil
}

func setNFTablesInputConnmarkRules(conn *nftables.Conn, table *nftables.Table, link netlink.Link) error {
	input := &nftables.Chain{
		Name:     strings.ToLower(inputChain),
		Table:    table,
		Type:     nftables.ChainTypeFilter,
		Hooknum:  nftables.ChainHookInput,
		Priority: nftables.ChainPriorityMangle,
		Policy:   func() *nftables.ChainPolicy { p := nftables.ChainPolicyAccept; return &p }(),
	}

	input, err := addNFTablesChainIfNotExists(conn, input)
	if err != nil {
		return fmt.Errorf("failed to add chain: %w", err)
	}

	devices := &nftables.Set{
		Table:        table,
		Name:         link.Attrs().Name,
		KeyType:      nftables.TypeIFName,
		KeyByteOrder: binaryutil.NativeEndian,
	}

	elements := []nftables.SetElement{
		{
			Key: ifname(link.Attrs().Name),
		},
	}

	devices, err = addNFTablesSetIfNotExists(conn, table, devices, elements)
	if err != nil {
		return fmt.Errorf("failed to add set: %w", err)
	}

	ruleID := nftRuleIDInputPrefix + link.Attrs().Name
	mark := &nftables.Rule{
		Table:    table,
		Chain:    input,
		UserData: []byte(ruleID),
		Exprs: []expr.Any{
			&expr.Meta{
				Key:      expr.MetaKeyIIFNAME,
				Register: 1,
			},
			&expr.Lookup{
				SetName:        devices.Name,
				SetID:          devices.ID,
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
				SourceRegister: true,
				Register:       1,
			},
		},
	}

	if _, err := addNFTablesRuleIfNotExists(conn, mark); err != nil {
		return fmt.Errorf("faild to check input rule existance: %w", err)
	}
	return nil
}

func setNFTablesOutputConnmarkRules(conn *nftables.Conn, table *nftables.Table, link netlink.Link) error {
	output := &nftables.Chain{
		Name:     outputChain,
		Table:    table,
		Type:     nftables.ChainTypeRoute,
		Hooknum:  nftables.ChainHookOutput,
		Priority: nftables.ChainPriorityMangle,
		Policy:   func() *nftables.ChainPolicy { p := nftables.ChainPolicyAccept; return &p }(),
	}

	output, err := addNFTablesChainIfNotExists(conn, output)
	if err != nil {
		return fmt.Errorf("failed to add chain: %w", err)
	}

	ruleID := nftRuleIDOutputPrefix + link.Attrs().Name
	restoreMark := &nftables.Rule{
		Table:    table,
		Chain:    output,
		UserData: []byte(ruleID),
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

	if _, err := addNFTablesRuleIfNotExists(conn, restoreMark); err != nil {
		return fmt.Errorf("faild to check input rule existance: %w", err)
	}
	return nil
}

func removeNFTablesConnmarkRules(family int, link netlink.Link) error {
	conn, err := nftables.New()
	if err != nil {
		return fmt.Errorf("failed to create nftables connection: %w", err)
	}
	nf, err := netlinkToNFTablesFamily(family)
	if err != nil {
		return err
	}

	// Use ListTables and filter by name, as ListTableOfFamily fails
	// in certain conditions (fresh namespaces or when table doesn't exist).
	tables, err := conn.ListTables()
	if err != nil {
		// If we can't list tables, the mangle table probably doesn't exist
		return nil
	}

	var mangle *nftables.Table
	for _, t := range tables {
		if t.Name == mangleTable && t.Family == nf {
			mangle = t
			break
		}
	}

	if mangle == nil {
		// Table doesn't exist, nothing to remove
		return nil
	}

	conn.DelTable(mangle)
	if err := conn.Flush(); err != nil {
		return fmt.Errorf("failed to flush nft rules: %w", err)
	}
	return nil
}

func addNFTablesChainIfNotExists(conn *nftables.Conn, chain *nftables.Chain) (*nftables.Chain, error) {
	// AddChain is idempotent - it will create the chain if it doesn't exist,
	// or return a reference to add to the existing chain if it does.
	// Reading the chain first (with ListChain) fails in fresh namespaces
	// before any flush has happened, so we just add directly.
	c := conn.AddChain(chain)
	return c, nil
}

func addNFTablesSetIfNotExists(conn *nftables.Conn, table *nftables.Table, set *nftables.Set, elements []nftables.SetElement) (*nftables.Set, error) {
	// AddSet is idempotent - it will create the set if it doesn't exist,
	// or be a no-op if it already exists.
	// Reading the set first (with GetSetByName) fails in fresh namespaces
	// before any flush has happened, so we just add directly.
	if err := conn.AddSet(set, elements); err != nil {
		return nil, fmt.Errorf("failed to add set %s : %w", set.Name, err)
	}
	return set, nil
}

func addNFTablesRuleIfNotExists(conn *nftables.Conn, target *nftables.Rule) (*nftables.Rule, error) {
	rs, err := conn.GetRules(target.Table, target.Chain)
	if err != nil {
		// In fresh namespaces (before any flush), GetRules may fail with
		// "netlink receive: no such file or directory". This is expected
		// and means there are no existing rules to deduplicate against.
		// We proceed to add the rule in this case.
		// On subsequent calls (after flush), GetRules should succeed
		// and we can properly deduplicate.
		return conn.AddRule(target), nil
	}

	for _, r := range rs {
		if same := compareNFTRules(r, target); same {
			return r, nil
		}
	}

	return conn.AddRule(target), nil
}

func compareNFTRules(a, b *nftables.Rule) bool {
	return bytes.Equal(a.UserData, b.UserData)
}

func netlinkToNFTablesFamily(family int) (nftables.TableFamily, error) {
	switch family {
	case netlink.FAMILY_V4:
		return nftables.TableFamilyIPv4, nil
	case netlink.FAMILY_V6:
		return nftables.TableFamilyIPv6, nil
	default:
		return 0, fmt.Errorf("invalid IP family %d", family)
	}
}

func ifname(n string) []byte {
	b := make([]byte, 16)
	copy(b, []byte(n+"\x00"))
	return b
}
