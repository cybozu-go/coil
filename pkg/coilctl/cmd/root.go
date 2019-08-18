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
	"fmt"
	"os"
	"strings"

	"github.com/cybozu-go/coil"
	"github.com/cybozu-go/etcdutil"
	"github.com/cybozu-go/log"
	"github.com/cybozu-go/well"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string
var etcdConfig *etcdutil.Config

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "coilctl",
	Short: "control and show coil settings",
	Long: `coilctl is a command-line tool to control and show coil settings.

It directly communicates with etcd.  You need to prepare a
configuration YAML to supply etcd connection parameters.
The default location of YAML is "$HOME/.coilctl.yml".`,

	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		err := well.LogConfig{}.Apply()
		if err != nil {
			log.ErrorExit(err)
		}

		jsonTagOption := func(c *mapstructure.DecoderConfig) {
			c.TagName = "json"
		}
		viper.Unmarshal(etcdConfig, jsonTagOption)

		err = coil.ResolveEtcdEndpoints(etcdConfig)
		if err != nil {
			log.ErrorExit(err)
		}

		log.Debug("etcd-config", map[string]interface{}{
			"config": fmt.Sprintf("%+v", etcdConfig),
		})
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
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.coilctl.yml)")

	// etcd connection parameters
	etcdConfig = coil.NewEtcdConfig()
	etcdConfig.AddPFlags(rootCmd.PersistentFlags())
	for _, key := range []string{"prefix", "endpoints", "timeout", "username", "password"} {
		viper.BindPFlag(key, rootCmd.PersistentFlags().Lookup("etcd-"+key))
	}
	for _, key := range []string{"tls-ca", "tls-cert", "tls-key"} {
		viper.BindPFlag(key+"-file", rootCmd.PersistentFlags().Lookup("etcd-"+key))
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".coilctl" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".coilctl")
	}

	// Read in environment variables that have the prefix "COILCTL_".
	viper.SetEnvPrefix("coilctl")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	viper.ReadInConfig()
}
