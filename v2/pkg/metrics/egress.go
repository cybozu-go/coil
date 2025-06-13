package metrics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"github.com/prometheus/client_golang/prometheus"
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
)

type egressCollector struct {
	conn             *nftables.Conn
	nfConntrackCount prometheus.Gauge
	nfConntrackLimit prometheus.Gauge
	perProtocol      map[string]*nfTablesPerProtocolMetrics
}

type nfTablesPerProtocolMetrics struct {
	NATPackets     prometheus.Gauge
	NATBytes       prometheus.Gauge
	InvalidPackets prometheus.Gauge
	InvalidBytes   prometheus.Gauge
}

func newNfTablesPerProtocolMetrics(ns, pod, egress, protocol string) *nfTablesPerProtocolMetrics {
	return &nfTablesPerProtocolMetrics{
		NATPackets:     NfTableMasqueradePackets.WithLabelValues(ns, pod, egress, protocol),
		NATBytes:       NfTableMasqueradeBytes.WithLabelValues(ns, pod, egress, protocol),
		InvalidPackets: NfTableInvalidPackets.WithLabelValues(ns, pod, egress, protocol),
		InvalidBytes:   NfTableInvalidBytes.WithLabelValues(ns, pod, egress, protocol),
	}
}

func NewEgressCollector(ns, pod, egress string, protocols []string) (Collector, error) {
	NfConntrackCount.Reset()
	NfConntrackLimit.Reset()
	NfTableMasqueradeBytes.Reset()
	NfTableMasqueradePackets.Reset()
	NfTableInvalidPackets.Reset()
	NfTableInvalidBytes.Reset()

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
	for _, rule := range rules {
		for _, e := range rule.Exprs {
			if counter, ok := e.(*expr.Counter); ok {
				// A rule in the egress pod must be only one, so we can return by finding a first one.
				return counter.Packets, counter.Bytes, nil
			}
		}
	}
	return 0, 0, errors.New("a rule for invalid packets is not found")
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
