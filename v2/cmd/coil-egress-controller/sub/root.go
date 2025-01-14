package sub

import (
	"flag"
	"fmt"
	"os"

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
	egressPort  int32
	zapOpts     zap.Options
}

var rootCmd = &cobra.Command{
	Use:     "coil-egress-controller",
	Short:   "controller for coil egress related custom resources",
	Long:    `coil-egress-controller is a Kubernetes controller for coil egress related custom resources.`,
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
	pf.StringVar(&config.metricsAddr, "metrics-addr", ":9396", "bind address of metrics endpoint")
	pf.StringVar(&config.healthAddr, "health-addr", ":9397", "bind address of health/readiness probes")
	pf.StringVar(&config.webhookAddr, "webhook-addr", ":9444", "bind address of admission webhook")
	pf.StringVar(&config.certDir, "cert-dir", "/certs", "directory to locate TLS certs for webhook")
	pf.Int32Var(&config.egressPort, "egress-port", 5555, "UDP port number used by coil-egress")

	goflags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(goflags)
	config.zapOpts.BindFlags(goflags)

	pf.AddGoFlagSet(goflags)
}
