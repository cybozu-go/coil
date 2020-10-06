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
	"sort"
	"strings"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
)

func showPool(name string, p *coil.AddressPool) {
	subnets := make([]string, len(p.Subnets))
	for i, subnet := range p.Subnets {
		subnets[i] = subnet.String()
	}
	fmt.Printf(`%s:
    Subnets: %s
    Size: %d
`, name, strings.Join(subnets, ", "), p.BlockSize)
}

// poolListCmd represents the list command
var poolListCmd = &cobra.Command{
	Use:   "list",
	Short: "list pool names and subnets",
	Long:  `list pool names and subnets`,
	Run: func(cmd *cobra.Command, args []string) {
		etcd, err := etcdutil.NewClient(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}
		defer etcd.Close()

		m := model.NewEtcdModel(etcd)
		well.Go(func(ctx context.Context) error {
			pools, err := m.ListPools(ctx)
			if err != nil {
				return err
			}

			names := make([]string, 0, len(pools))
			for k := range pools {
				names = append(names, k)
			}
			sort.Strings(names)

			for _, n := range names {
				showPool(n, pools[n])
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
	poolCmd.AddCommand(poolListCmd)
}
