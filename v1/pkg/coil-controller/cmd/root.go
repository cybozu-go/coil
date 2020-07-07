package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cybozu-go/coil/v1"
	"github.com/cybozu-go/coil/v1/controller"
	"github.com/cybozu-go/coil/v1/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
)

const (
	defaultScanInterval      = time.Minute * 10
	defaultAddressExpiration = time.Hour * 24
)

var config struct {
	scanInterval      time.Duration
	addressExpiration time.Duration
}

var etcdConfig *etcdutil.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "coil-controller",
	Version: coil.Version,
	Short:   "A kubernetes controller for coil",
	Long: `coil-controller is a Kubernetes controller to maintain coil resources.

It should be deployed as a Deployment pod in Kunernetes.
`,

	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		err := well.LogConfig{}.Apply()
		if err != nil {
			log.ErrorExit(err)
		}
	},

	Run: func(cmd *cobra.Command, args []string) {
		err := coil.ResolveEtcdEndpoints(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}

		etcd, err := etcdutil.NewClient(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}
		defer etcd.Close()

		db := model.NewEtcdModel(etcd)
		cntl, err := controller.NewController(db)
		if err != nil {
			log.ErrorExit(err)
		}

		well.Go(func(ctx context.Context) error {
			rev, err := cntl.Sync(ctx)
			if err != nil {
				return err
			}

			return cntl.Watch(ctx, rev)
		})

		well.Go(func(ctx context.Context) error {
			return cntl.ScanLoop(ctx, config.scanInterval, config.addressExpiration)
		})

		err = well.Wait()
		if err != nil && !well.IsSignaled(err) {
			log.ErrorExit(err)
		}
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
	etcdConfig = coil.NewEtcdConfig()
	etcdConfig.AddPFlags(rootCmd.PersistentFlags())

	fs := rootCmd.Flags()
	fs.DurationVar(&config.scanInterval, "scan-interval", defaultScanInterval, "Scan interval of IP address inconsistency")
	fs.DurationVar(&config.addressExpiration, "address-expiration", defaultAddressExpiration, "Expiration for alerting unused address")
}
