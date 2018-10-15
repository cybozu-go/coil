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
	"errors"
	"net"
	"regexp"
	"strconv"

	mycmd "github.com/cybozu-go/cmd"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/spf13/cobra"
)

var (
	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

	createParams struct {
		Name   string
		Subnet *net.IPNet
		Size   int
	}
)

// poolCreateCmd represents the create command
var poolCreateCmd = &cobra.Command{
	Use:   "create NAME SUBNET SIZE",
	Short: "creates an address pool",
	Long: `Creates an address pool.

Coil requires at least "default" address pool.
The other pools are pods running in the namespace of the same name.`,

	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 3 {
			return errors.New("requires 3 argument")
		}

		if !namePattern.MatchString(args[0]) {
			return errors.New("bad pool name")
		}

		ip, subnet, err := net.ParseCIDR(args[1])
		if err != nil {
			return err
		}

		size, err := strconv.Atoi(args[2])
		if err != nil {
			return err
		}

		if size < 0 {
			return errors.New("invalid size")
		}
		if ip.To4() != nil {
			if size > 24 {
				return errors.New("invalid size")
			}
		} else {
			if size > 120 {
				return errors.New("invalid size")
			}
		}

		createParams.Name = args[0]
		createParams.Subnet = subnet
		createParams.Size = size
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
			return m.AddPool(ctx, createParams.Name, createParams.Subnet, createParams.Size)
		})
		mycmd.Stop()
		err = mycmd.Wait()
		if err != nil {
			log.ErrorExit(err)
		}
	},
}

func init() {
	poolCmd.AddCommand(poolCreateCmd)
}
