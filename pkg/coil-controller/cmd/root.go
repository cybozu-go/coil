package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/controller"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
)

var etcdConfig *etcdutil.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "coil-controller",
	Short: "A kubernetes controller for coil",
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
}
