// Copyright © 2018 Cybozu
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	mycmd "github.com/cybozu-go/cmd"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/spf13/cobra"
)

var showParams struct {
	JSON   bool
	Name   string
	Subnet *net.IPNet
}

// poolShowCmd represents the create command
var poolShowCmd = &cobra.Command{
	Use:   "show NAME SUBNET",
	Short: "shows block assignment information of a subnet",
	Long:  `Shows block assignment information of a subnet`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return errors.New("requires 2 argument")
		}

		_, subnet, err := net.ParseCIDR(args[1])
		if err != nil {
			return err
		}

		showParams.Name = args[0]
		showParams.Subnet = subnet
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		etcd, err := etcdutil.NewClient(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}
		defer etcd.Close()

		m := model.NewEtcdModel(etcd)
		mycmd.Go(func(ctx context.Context) error {
			ba, err := m.GetAssignments(ctx, showParams.Name, showParams.Subnet)
			if err != nil {
				return err
			}
			if showParams.JSON {
				return json.NewEncoder(os.Stdout).Encode(ba)
			}

			free := len(ba.FreeList)
			total := free
			for _, v := range ba.Nodes {
				total = total + len(v)
			}
			fmt.Printf("free blocks: %d out of %d\n", free, total)
			return nil
		})
		mycmd.Stop()
		err = mycmd.Wait()
		if err != nil {
			log.ErrorExit(err)
		}
	},
}

func init() {
	poolCmd.AddCommand(poolShowCmd)
	poolShowCmd.Flags().BoolVar(&showParams.JSON, "json", false, "show in JSON")
}
