package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"

	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
)

var infoParams struct {
	Address net.IP
}

// addressInfoCmd represents the info command
var addressInfoCmd = &cobra.Command{
	Use:   "info ADDRESS",
	Short: "shows ID of the container to which ADDRESS is assigned",
	Long:  `Shows ID of the container to which ADDRESS is assigned`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("requires 1 argument")
		}

		infoParams.Address = net.ParseIP(args[0])

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
			assignment, _, err := m.GetAddressInfo(ctx, infoParams.Address)
			if err != nil {
				return err
			}

			data, err := json.Marshal(assignment)
			if err != nil {
				return err
			}

			e := json.NewEncoder(os.Stdout)
			e.SetIndent("", "  ")
			return e.Encode(data)
		})
		well.Stop()
		err = well.Wait()
		if err != nil {
			log.ErrorExit(err)
		}
	},
}

func init() {
	addressCmd.AddCommand(addressInfoCmd)
}
