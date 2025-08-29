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
	metricsAddr     string
	healthAddr      string
	port            int
	enableSportAuto bool
	backend         string
	zapOpts         zap.Options
}

var rootCmd = &cobra.Command{
	Use:     "coil-egress",
	Short:   "manage foo-over-udp tunnels in egress pods",
	Long:    `coil-egress manages Foo-over-UDP tunnels in pods created by Egress.`,
	Version: v2.Version(),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if config.backend != constants.BackendIPTables && config.backend != constants.BackendNFTables {
			return fmt.Errorf("invalid backend: %s (must be either %s or %s)",
				config.backend, constants.BackendIPTables, constants.BackendNFTables)
		}
		return nil
	},
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
	pf.StringVar(&config.metricsAddr, "metrics-addr", ":8080", "bind address of metrics endpoint")
	pf.StringVar(&config.healthAddr, "health-addr", ":8081", "bind address of health/readiness probes")
	pf.IntVar(&config.port, "fou-port", 5555, "port number for foo-over-udp tunnels")
	pf.BoolVar(&config.enableSportAuto, "enable-sport-auto", false, "enable automatic source port assignment")
	pf.StringVar(&config.backend, "backend", constants.DefaultBackend, "Backend for egress NAT rules: iptables or nftables (default: iptables)")

	goflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(goflags)
	config.zapOpts.BindFlags(goflags)

	pf.AddGoFlagSet(goflags)
}
