package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
	"github.com/tcnksm/go-input"
)

var (
	freeParams struct {
		Address net.IP
	}
	yorn string
)

func deleteInput() error {
	ui := input.DefaultUI()
	validateFunc := func(s string) error {
		if s != "y" && s != "n" {
			return fmt.Errorf("input must be y or n")
		}
		return nil
	}

	ask := func(p *string, query string, mask bool, validate func(s string) error) error {
		ans, err := ui.Ask(query, &input.Options{
			Default:      "n",
			Required:     true,
			Loop:         true,
			MaskDefault:  mask,
			ValidateFunc: validate,
		})
		if err != nil {
			return err
		}
		*p = strings.TrimSpace(ans)
		return nil
	}

	if err := ask(&yorn, "are you sure to delete?", false, validateFunc); err != nil {
		return err
	}
	return nil
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

			assignment, modRev, err := m.GetAddressInfo(ctx, freeParams.Address)
			if err != nil {
				return err
			}

			data, err := json.Marshal(assignment)
			if err != nil {
				return err
			}

			e := json.NewEncoder(os.Stdout)
			e.SetIndent("", "  ")
			err = e.Encode(data)
			if err != nil {
				return err
			}

			if err = deleteInput(); err != nil {
				return err
			}

			if yorn == "y" {
				return m.FreeIP(ctx, myBlock, freeParams.Address, modRev)
			}

			return nil
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
