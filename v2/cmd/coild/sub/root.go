package sub

import (
	"flag"
	"fmt"
	"os"

	v2 "github.com/cybozu-go/coil/v2"
	"github.com/cybozu-go/coil/v2/pkg/constants"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var config struct {
	metricsAddr      string
	healthAddr       string
	podTableId       int
	podRulePrio      int
	exportTableId    int
	protocolId       int
	socketPath       string
	compatCalico     bool
	egressPort       int
	registerFromMain bool
	zapOpts          zap.Options
	enableIPAM       bool
	enableEgress     bool
}

var rootCmd = &cobra.Command{
	Use:   "coild",
	Short: "gRPC server running on each node",
	Long: `coild is a gRPC server running on each node.

It listens on a UNIX domain socket and accepts requests from
coil CNI plugin.`,
	Version: v2.Version(),
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		return subMain()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.StringVar(&config.metricsAddr, "metrics-addr", ":9384", "bind address of metrics endpoint")
	pf.StringVar(&config.healthAddr, "health-addr", ":9385", "bind address of health/readiness probes")
	pf.IntVar(&config.podTableId, "pod-table-id", 116, "routing table ID to which coild registers routes for Pods")
	pf.IntVar(&config.podRulePrio, "pod-rule-prio", 2000, "priority with which the rule for Pod table is inserted")
	pf.IntVar(&config.exportTableId, "export-table-id", 119, "routing table ID to which coild exports routes")
	pf.IntVar(&config.protocolId, "protocol-id", 30, "route author ID")
	pf.StringVar(&config.socketPath, "socket", constants.DefaultSocketPath, "UNIX domain socket path")
	pf.BoolVar(&config.compatCalico, "compat-calico", false, "make veth name compatible with Calico")
	pf.IntVar(&config.egressPort, "egress-port", 5555, "UDP port number for egress NAT")
	pf.BoolVar(&config.registerFromMain, "register-from-main", false, "help migration from Coil 2.0.1")
	pf.BoolVar(&config.enableIPAM, "enable-ipam", true, "enable IPAM related features")
	pf.BoolVar(&config.enableEgress, "enable-egress", true, "enable IPAM related features")

	goflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(goflags)
	config.zapOpts.BindFlags(goflags)

	pf.AddGoFlagSet(goflags)
}
