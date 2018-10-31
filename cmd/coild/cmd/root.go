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
	"net/http"
	"os"
	"time"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/coil/coild"
	"github.com/cybozu-go/coil/model"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	"github.com/spf13/cobra"
)

const (
	defaultListenHTTP = "127.0.0.1:9383"
	defaultTableID    = 119
	defaultProtocolID = 30
)

var config struct {
	endpoint   string
	tableID    int
	protocolID int
}

var etcdConfig *etcdutil.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "coild",
	Short: "Backend service to allocate IP address",
	Long: `coild is a backend service to allocate IP address.

It normally run as a DaemonSet container in Kubernetes.
Following environment variables need to be set:

    COIL_NODE_NAME:  The node name where the container is running.
    COIL_NODE_IP:    A routable IP address to the node.
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
		server := coild.NewServer(db, config.tableID, config.protocolID)

		well.Go(func(ctx context.Context) error {
			return subMain(ctx, server)
		})
		err = well.Wait()
		if err != nil && !well.IsSignaled(err) {
			log.ErrorExit(err)
		}
	},
}

func subMain(ctx context.Context, server *coild.Server) error {
	err := server.Init(ctx)
	if err != nil {
		return nil
	}

	webServer := &well.HTTPServer{
		Server: &http.Server{
			Addr:    config.endpoint,
			Handler: server,
		},
		ShutdownTimeout: 3 * time.Minute,
	}
	webServer.ListenAndServe()
	return nil
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
	fs.StringVar(&config.endpoint, "http", defaultListenHTTP, "REST API endpoint")
	fs.IntVar(&config.tableID, "table-id", defaultTableID, "Routing table ID to export routes")
	fs.IntVar(&config.protocolID, "protocol-id", defaultProtocolID, "Route author ID")
}
