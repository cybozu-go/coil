package sub

import (
	"flag"
	"fmt"
	"os"
	"time"

	v2 "github.com/cybozu-go/coil/v2"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var config struct {
	metricsAddr string
	healthAddr  string
	webhookAddr string
	certDir     string
	gcInterval  time.Duration
	zapOpts     zap.Options
}

var rootCmd = &cobra.Command{
	Use:     "coil-controller",
	Short:   "controller for coil custom resources",
	Long:    `coil-controller is a Kubernetes controller for coil custom resources.`,
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
	pf.StringVar(&config.metricsAddr, "metrics-addr", ":9386", "bind address of metrics endpoint")
	pf.StringVar(&config.healthAddr, "health-addr", ":9387", "bind address of health/readiness probes")
	pf.StringVar(&config.webhookAddr, "webhook-addr", ":9443", "bind address of admission webhook")
	pf.StringVar(&config.certDir, "cert-dir", "/certs", "directory to locate TLS certs for webhook")
	pf.DurationVar(&config.gcInterval, "gc-interval", 1*time.Hour, "garbage collection interval")

	goflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(goflags)
	config.zapOpts.BindFlags(goflags)

	pf.AddGoFlagSet(goflags)
}
