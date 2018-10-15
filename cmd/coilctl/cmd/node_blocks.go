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
	"fmt"

	mycmd "github.com/cybozu-go/cmd"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/spf13/cobra"
)

// nodeBlocksCmd represents the blocks command
var nodeBlocksCmd = &cobra.Command{
	Use:   "blocks NODE",
	Short: "List all address blocks assigned to a node",
	Long:  `List all address blocks assigned to a node.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		node := args[0]

		etcd, err := etcdutil.NewClient(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}
		defer etcd.Close()

		m := model.NewEtcdModel(etcd)
		mycmd.Go(func(ctx context.Context) error {
			blocks, err := m.GetMyBlocks(ctx, node)
			if err != nil {
				return err
			}
			for k, v := range blocks {
				fmt.Printf("pool: %s\n", k)
				for _, block := range v {
					fmt.Printf("    %v\n", block)
				}
			}
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
	nodeCmd.AddCommand(nodeBlocksCmd)
}
