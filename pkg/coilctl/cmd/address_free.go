package cmd

import (
	"context"
	"errors"
	"net"

	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
)

var freeParams struct {
	Address net.IP
}

// addressFreeCmd represents the free command
var addressFreeCmd = &cobra.Command{
	Use:   "free ADDRESS",
	Short: "free ADDRESS regardless of Pods statuses",
	Long:  `Free ADDRESS regardless of Pods statuses`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("requires 1 argument")
		}

		freeParams.Address = net.ParseIP(args[0])

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		etcd, err := etcdutil.NewClient(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}
		defer etcd.Close()

		m := model.NewEtcdModel(etcd)
		well.Go(func(ctx context.Context) error {
			blocksMap, err := m.GetAssignedBlocks(ctx)
			if err != nil {
				return err
			}

			var myBlock *net.IPNet
			for _, blocks := range blocksMap {
				for _, block := range blocks {
					if block.Contains(freeParams.Address) {
						myBlock = block
					}
				}
			}
			if myBlock == nil {
				return errors.New("ADDRESS is not assigned")
			}

			return m.FreeIP(ctx, myBlock, freeParams.Address)
		})
		well.Stop()
		err = well.Wait()
		if err != nil {
			log.ErrorExit(err)
		}
	},
}

func init() {
	addressCmd.AddCommand(addressFreeCmd)
}
