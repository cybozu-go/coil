package config

import (
	"flag"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/cybozu-go/coil/v2/pkg/constants"
)

type Config struct {
	MetricsAddr            string
	HealthAddr             string
	PodTableId             int
	PodRulePrio            int
	ExportTableId          int
	ProtocolId             int
	SocketPath             string
	CompatCalico           bool
	EgressPort             int
	RegisterFromMain       bool
	ZapOpts                zap.Options
	EnableIPAM             bool
	EnableEgress           bool
	AddressBlockGCInterval time.Duration
	Backend                string
}

func Parse(rootCmd *cobra.Command) *Config {
	config := &Config{}
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&config.MetricsAddr, "metrics-addr", constants.DefautlMetricsAddr, "bind address of metrics endpoint")
	pf.StringVar(&config.HealthAddr, "health-addr", constants.DefautlHealthAddr, "bind address of health/readiness probes")
	pf.IntVar(&config.PodTableId, "pod-table-id", constants.DefautlPodTableId, "routing table ID to which coild registers routes for Pods")
	pf.IntVar(&config.PodRulePrio, "pod-rule-prio", constants.DefautlPodRulePrio, "priority with which the rule for Pod table is inserted")
	pf.IntVar(&config.ExportTableId, "export-table-id", constants.DefautlExportTableId, "routing table ID to which coild exports routes")
	pf.IntVar(&config.ProtocolId, "protocol-id", constants.DefautlProtocolId, "route author ID")
	pf.StringVar(&config.SocketPath, "socket", constants.DefaultSocketPath, "UNIX domain socket path")
	pf.BoolVar(&config.CompatCalico, "compat-calico", constants.DefaultCompatCalico, "make veth name compatible with Calico")
	pf.IntVar(&config.EgressPort, "egress-port", constants.DefaultEgressPort, "UDP port number for egress NAT")
	pf.BoolVar(&config.RegisterFromMain, "register-from-main", constants.DefaultRegisterFromMain, "help migration from Coil 2.0.1")
	pf.BoolVar(&config.EnableIPAM, "enable-ipam", constants.DefaultEnableIPAM, "enable IPAM related features")
	pf.BoolVar(&config.EnableEgress, "enable-egress", constants.DefaultEnableEgress, "enable Egress related features")
	pf.DurationVar(&config.AddressBlockGCInterval, "addressblock-gc-interval", constants.DefaultAddressBlockGCInterval, "interval for address block GC")
	pf.StringVar(&config.Backend, "backend", constants.DefaultEgressBackend, "backend for egress NAT rules: iptables or nftables (default: iptables)")

	goflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(goflags)
	config.ZapOpts.BindFlags(goflags)

	pf.AddGoFlagSet(goflags)

	return config
}
