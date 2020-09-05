package sub

import (
	"fmt"
	"os"
	"time"

	v2 "github.com/cybozu-go/coil/v2"
	"github.com/spf13/cobra"
)

var config struct {
	metricsAddr    string
	healthAddr     string
	protocolId     int
	updateInterval time.Duration
}

var rootCmd = &cobra.Command{
	Use:   "coil-router",
	Short: "a simple routing program for Coil",
	Long: `coil-router programs Linux kernel routing table to route
Pod packets between nodes.

coil-router does not speak any routing protocol such as BGP.
Instead, it directly insert routes corresponding to AddressBlocks
owned by other Nodes.  This means that coil-router can be used
only for clusters where all the nodes are in a flat L2 network.`,
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
	pf.StringVar(&config.metricsAddr, "metrics-addr", ":9388", "bind address of metrics endpoint")
	pf.StringVar(&config.healthAddr, "health-addr", ":9389", "bind address of health/readiness probes")
	pf.IntVar(&config.protocolId, "protocol-id", 31, "route author ID")
	pf.DurationVar(&config.updateInterval, "update-interval", 10*time.Minute, "interval for forced route update")
}
