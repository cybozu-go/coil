package metrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/google/nftables/xt"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/cybozu-go/coil/v2/pkg/nat/netfilter"
)

const (
	NF_CONNTRACK_COUNT_PATH = "/proc/sys/net/netfilter/nf_conntrack_count"
	NF_CONNTRACK_LIMIT_PATH = "/proc/sys/net/netfilter/nf_conntrack_max"
)

var (
	NfConntrackCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nf_conntrack_entries_count",
		Help:      "the number of entries in the nf_conntrack table",
	}, []string{"namespace", "pod", "egress"})

	NfConntrackLimit = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nf_conntrack_entries_limit",
		Help:      "the limit of the nf_conntrack table",
	}, []string{"namespace", "pod", "egress"})

	NfTableMasqueradePackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_masqueraded_packets_total",
		Help:      "the number of packets that are masqueraded by nftables",
	}, []string{"namespace", "pod", "egress", "protocol"})

	NfTableMasqueradeBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_masqueraded_bytes_total",
		Help:      "the number of bytes that are masqueraded by nftables",
	}, []string{"namespace", "pod", "egress", "protocol"})

	NfTableInvalidPackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_invalid_packets_total",
		Help:      "the number of packets that are dropped as invalid packets by nftables",
	}, []string{"namespace", "pod", "egress", "protocol"})

	NfTableInvalidBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_invalid_bytes_total",
		Help:      "the number of bytes that are dropped as invalid packets by nftables",
	}, []string{"namespace", "pod", "egress", "protocol"})

	NfTablePeerSrcDroppedPackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_peer_src_dropped_packets_total",
		Help:      "the number of packets from clients that are dropped by nftables because of unexpected source addresses",
	}, []string{"namespace", "pod", "egress", "protocol"})

	NfTablePeerSrcDroppedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_peer_src_dropped_bytes_total",
		Help:      "the number of bytes from clients that are dropped by nftables because of unexpected source addresses",
	}, []string{"namespace", "pod", "egress", "protocol"})
)

type egressCollector struct {
	conn             *nftables.Conn
	nfConntrackCount prometheus.Gauge
	nfConntrackLimit prometheus.Gauge
	perProtocol      map[string]*nfTablesPerProtocolMetrics
}

type nfTablesPerProtocolMetrics struct {
	NATPackets            prometheus.Gauge
	NATBytes              prometheus.Gauge
	InvalidPackets        prometheus.Gauge
	InvalidBytes          prometheus.Gauge
	PeerSrcDroppedPackets prometheus.Gauge
	PeerSrcDroppedBytes   prometheus.Gauge
	peerSrc               *peerSrcCounter
}

func newNfTablesPerProtocolMetrics(ns, pod, egress, protocol string) *nfTablesPerProtocolMetrics {
	return &nfTablesPerProtocolMetrics{
		NATPackets:            NfTableMasqueradePackets.WithLabelValues(ns, pod, egress, protocol),
		NATBytes:              NfTableMasqueradeBytes.WithLabelValues(ns, pod, egress, protocol),
		InvalidPackets:        NfTableInvalidPackets.WithLabelValues(ns, pod, egress, protocol),
		InvalidBytes:          NfTableInvalidBytes.WithLabelValues(ns, pod, egress, protocol),
		PeerSrcDroppedPackets: NfTablePeerSrcDroppedPackets.WithLabelValues(ns, pod, egress, protocol),
		PeerSrcDroppedBytes:   NfTablePeerSrcDroppedBytes.WithLabelValues(ns, pod, egress, protocol),
		peerSrc:               &peerSrcCounter{},
	}
}

func NewEgressCollector(ns, pod, egress string, protocols []string) (Collector, error) {
	NfConntrackCount.Reset()
	NfConntrackLimit.Reset()
	NfTableMasqueradeBytes.Reset()
	NfTableMasqueradePackets.Reset()
	NfTableInvalidPackets.Reset()
	NfTableInvalidBytes.Reset()
	NfTablePeerSrcDroppedPackets.Reset()
	NfTablePeerSrcDroppedBytes.Reset()

	c, err := nftables.New()
	if err != nil {
		return nil, err
	}

	perProtocols := make(map[string]*nfTablesPerProtocolMetrics)
	for _, protocol := range protocols {
		perProtocols[protocol] = newNfTablesPerProtocolMetrics(ns, pod, egress, protocol)
	}

	return &egressCollector{
		conn:             c,
		nfConntrackCount: NfConntrackCount.WithLabelValues(ns, pod, egress),
		nfConntrackLimit: NfConntrackLimit.WithLabelValues(ns, pod, egress),
		perProtocol:      perProtocols,
	}, nil
}

func (c *egressCollector) Name() string {
	return "egress-collector"
}

func (c *egressCollector) Update(ctx context.Context) error {
	val, err := readUintFromFile(NF_CONNTRACK_COUNT_PATH)
	if err != nil {
		return err
	}
	c.nfConntrackCount.Set(float64(val))

	val, err = readUintFromFile(NF_CONNTRACK_LIMIT_PATH)
	if err != nil {
		return err
	}
	c.nfConntrackLimit.Set(float64(val))

	for protocol, nfTablesMetrics := range c.perProtocol {
		natPackets, natBytes, err := c.getNfTablesNATCounter(protocol)
		if err != nil {
			return err
		}
		nfTablesMetrics.NATPackets.Set(float64(natPackets))
		nfTablesMetrics.NATBytes.Set(float64(natBytes))

		invalidPackets, invalidBytes, err := c.getNfTablesInvalidCounter(protocol)
		if err != nil {
			return err
		}
		nfTablesMetrics.InvalidPackets.Set(float64(invalidPackets))
		nfTablesMetrics.InvalidBytes.Set(float64(invalidBytes))

		peerSrcCounters, err := c.getNfTablesPeerSrcDroppedCounters(protocol)
		if err != nil {
			return err
		}
		peerSrcDropped := nfTablesMetrics.peerSrc.update(peerSrcCounters)
		nfTablesMetrics.PeerSrcDroppedPackets.Set(float64(peerSrcDropped.packets))
		nfTablesMetrics.PeerSrcDroppedBytes.Set(float64(peerSrcDropped.bytes))
	}

	return nil
}

func (c *egressCollector) getNfTablesNATCounter(protocol string) (uint64, uint64, error) {
	family, err := stringToTableFamily(protocol)
	if err != nil {
		return 0, 0, err
	}
	table := &nftables.Table{Family: family, Name: "nat"}
	rules, err := c.conn.GetRules(table, &nftables.Chain{
		Name:    "POSTROUTING",
		Type:    nftables.ChainTypeNAT,
		Table:   table,
		Hooknum: nftables.ChainHookPostrouting,
	})
	if err != nil {
		return 0, 0, err
	}
	for _, rule := range rules {
		for _, e := range rule.Exprs {
			if counter, ok := e.(*expr.Counter); ok {
				// A rule in the egress pod must be only one, so we can return by finding a first one.
				return counter.Packets, counter.Bytes, nil
			}
		}
	}
	return 0, 0, errors.New("a masquerade rule is not found")
}

func (c *egressCollector) getNfTablesInvalidCounter(protocol string) (uint64, uint64, error) {
	family, err := stringToTableFamily(protocol)
	if err != nil {
		return 0, 0, err
	}
	table := &nftables.Table{Family: family, Name: "filter"}
	rules, err := c.conn.GetRules(table, &nftables.Chain{
		Name:    "FORWARD",
		Type:    nftables.ChainTypeFilter,
		Table:   table,
		Hooknum: nftables.ChainHookForward,
	})
	if err != nil {
		return 0, 0, err
	}
	packets, bytes, found := sumCounters(rules, func(tag string) bool {
		return tag == netfilter.RuleTagInvalidDrop
	})
	if !found {
		return 0, 0, errors.New("a rule for invalid packets is not found")
	}
	return packets, bytes, nil
}

// getNfTablesPeerSrcDroppedCounters returns the counters of the rules that
// drop packets from clients with unexpected source addresses.  The rules
// exist in FORWARD and INPUT chains for each client, and there are none if
// no client has been added.  The counters are keyed by rule handles.
func (c *egressCollector) getNfTablesPeerSrcDroppedCounters(protocol string) (map[uint64]counterValue, error) {
	family, err := stringToTableFamily(protocol)
	if err != nil {
		return nil, err
	}
	// GetRules fails if the chain does not exist, e.g. INPUT chain before
	// any client is added, so get rules only from existing chains.
	chains, err := c.conn.ListChainsOfTableFamily(family)
	if err != nil {
		return nil, err
	}

	counters := make(map[uint64]counterValue)
	for _, chain := range chains {
		if chain.Table.Name != "filter" || (chain.Name != "FORWARD" && chain.Name != "INPUT") {
			continue
		}
		rules, err := c.conn.GetRules(chain.Table, chain)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			if !strings.HasPrefix(ruleTag(rule), netfilter.RuleTagPeerSrcPrefix) {
				continue
			}
			counters[rule.Handle] = ruleCounter(rule)
		}
	}
	return counters, nil
}

type counterValue struct {
	packets uint64
	bytes   uint64
}

// peerSrcCounter keeps the counts of removed coil-peer-src-* rules so that
// the sum does not go down when a client is deleted.  Packets dropped
// between the last update and the removal of a rule are not counted.
//
// Rules are identified by their handles.  Handles are unique in a table and
// not reused, so a rule re-created for the same client is a different one.
type peerSrcCounter struct {
	lastSeen map[uint64]counterValue
	removed  counterValue
}

// update records the current counters and returns the total including
// the counts of the rules removed so far.
func (p *peerSrcCounter) update(current map[uint64]counterValue) counterValue {
	for handle, v := range p.lastSeen {
		if _, ok := current[handle]; !ok {
			p.removed.packets += v.packets
			p.removed.bytes += v.bytes
		}
	}
	p.lastSeen = current

	total := p.removed
	for _, v := range current {
		total.packets += v.packets
		total.bytes += v.bytes
	}
	return total
}

// sumCounters sums the counters of the rules whose tags satisfy match.
// found reports whether any rule satisfies match.
func sumCounters(rules []*nftables.Rule, match func(tag string) bool) (packets, bytes uint64, found bool) {
	for _, rule := range rules {
		if !match(ruleTag(rule)) {
			continue
		}
		found = true
		v := ruleCounter(rule)
		packets += v.packets
		bytes += v.bytes
	}
	return packets, bytes, found
}

// ruleCounter returns the sum of the counters in a rule.
func ruleCounter(rule *nftables.Rule) counterValue {
	var v counterValue
	for _, e := range rule.Exprs {
		if counter, ok := e.(*expr.Counter); ok {
			v.packets += counter.Packets
			v.bytes += counter.Bytes
		}
	}
	return v
}

// ruleTag returns the tag of a rule set by coil.  The tag is stored as
// UserData for nftables, and as a comment match for iptables-nft.
func ruleTag(rule *nftables.Rule) string {
	if len(rule.UserData) != 0 {
		return string(rule.UserData)
	}
	for _, e := range rule.Exprs {
		m, ok := e.(*expr.Match)
		if !ok || m.Name != "comment" {
			continue
		}
		if c, ok := m.Info.(*xt.Comment); ok {
			return string(*c)
		}
	}
	return ""
}

func readUintFromFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

func stringToTableFamily(protocol string) (nftables.TableFamily, error) {
	switch protocol {
	case constants.FamilyIPv4:
		return nftables.TableFamilyIPv4, nil
	case constants.FamilyIPv6:
		return nftables.TableFamilyIPv6, nil
	default:
		return nftables.TableFamilyUnspecified, fmt.Errorf("unsupported family type: %s", protocol)
	}
}
