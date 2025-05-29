package metrics

import (
	"errors"
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
	NfConnctackCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nf_conntrack_entries_count",
		Help:      "the number of entries in the nf_conntrack table",
	}, []string{"namespace", "pod", "egress"})

	NfConnctackLimit = prometheus.NewGaugeVec(prometheus.GaugeOpts{
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
	}, []string{"namespace", "pod", "egress"})

	NfTableMasqueradeBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_masqueraded_bytes_total",
		Help:      "the number of bytes that are masqueraded by nftables",
	}, []string{"namespace", "pod", "egress"})

	NfTableInvalidPackets = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_invalid_packets_total",
		Help:      "the number of packets that are dropped as invalid packets by nftables",
	}, []string{"namespace", "pod", "egress"})

	NfTableInvalidBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: constants.MetricsNS,
		Subsystem: "egress",
		Name:      "nftables_invalid_bytes_total",
		Help:      "the number of bytes that are dropped as invalid packets by nftables",
	}, []string{"namespace", "pod", "egress"})
)

type egressCollector struct {
	conn                   *nftables.Conn
	nfConnctackCount       prometheus.Gauge
	nfConnctackLimit       prometheus.Gauge
	nfTablesNATPackets     prometheus.Gauge
	nfTablesNATBytes       prometheus.Gauge
	nfTablesInvalidPackets prometheus.Gauge
	nfTablesInvalidBytes   prometheus.Gauge
}

func NewEgressCollector(ns, pod, egress string) (Collector, error) {
	NfConnctackCount.Reset()
	NfConnctackLimit.Reset()
	NfTableMasqueradeBytes.Reset()
	NfTableMasqueradePackets.Reset()
	NfTableInvalidPackets.Reset()
	NfTableInvalidBytes.Reset()

	c, err := nftables.New()
	if err != nil {
		return nil, err
	}

	return &egressCollector{
		conn:                   c,
		nfConnctackCount:       NfConnctackCount.WithLabelValues(ns, pod, egress),
		nfConnctackLimit:       NfConnctackLimit.WithLabelValues(ns, pod, egress),
		nfTablesNATPackets:     NfTableMasqueradePackets.WithLabelValues(ns, pod, egress),
		nfTablesNATBytes:       NfTableMasqueradeBytes.WithLabelValues(ns, pod, egress),
		nfTablesInvalidPackets: NfTableInvalidPackets.WithLabelValues(ns, pod, egress),
		nfTablesInvalidBytes:   NfTableInvalidBytes.WithLabelValues(ns, pod, egress),
	}, nil
}

func (c *egressCollector) Name() string {
	return "egress-collector"
}

func (c *egressCollector) Update() error {

	val, err := readUintFromFile(NF_CONNTRACK_COUNT_PATH)
	if err != nil {
		return err
	}
	c.nfConnctackCount.Set(float64(val))

	val, err = readUintFromFile(NF_CONNTRACK_LIMIT_PATH)
	if err != nil {
		return err
	}
	c.nfConnctackLimit.Set(float64(val))

	natPackets, natBytes, err := c.getNfTablesNATCounter()
	if err != nil {
		return nil
	}
	c.nfTablesNATPackets.Set(float64(natPackets))
	c.nfTablesNATBytes.Set(float64(natBytes))

	invalidPackets, invalidBytes, err := c.getNfTablesInvalidCounter()
	if err != nil {
		return nil
	}
	c.nfTablesInvalidPackets.Set(float64(invalidPackets))
	c.nfTablesInvalidBytes.Set(float64(invalidBytes))

	return nil
}

func (c *egressCollector) getNfTablesNATCounter() (uint64, uint64, error) {
	table := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: "nat"}
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

func (c *egressCollector) getNfTablesInvalidCounter() (uint64, uint64, error) {
	table := &nftables.Table{Family: nftables.TableFamilyIPv4, Name: "filter"}
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
