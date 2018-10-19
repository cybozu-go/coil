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

	"github.com/cybozu-go/coil/installer"
	"github.com/cybozu-go/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	defaultCniConfName = "10-coil.conflist"
	defaultCniEtcDir   = "/host/etc/cni/net.d"
	defaultCniBinDir   = "/host/opt/cni/bin"
	defaultCoilPath    = "/coil"
	defaultBootTaint   = "coil.cybozu.com/bootstrap"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "coil-installer",
	Short: "coil-installer installs a CNI plugin coil and a CNI configuration file to host OS",
	Long: `coil-installer installs a CNI plugin coil and a CNI configuration
file to host OS by using DaemonSet pod. The pod has a container to install them
using given environment variables.`,
	Run: func(cmd *cobra.Command, args []string) {
		cniConfName := viper.GetString("CNI_CONF_NAME")
		cniEtcDir := viper.GetString("CNI_ETC_DIR")
		cniBinDir := viper.GetString("CNI_BIN_DIR")
		coilPath := viper.GetString("COIL_PATH")
		cniNetConf := viper.GetString("CNI_NETCONF")
		cniNetConfFile := viper.GetString("CNI_NETCONF_FILE")
		coilNodeName := viper.GetString("COIL_NODE_NAME")
		coilBootTaint := viper.GetString("COIL_BOOT_TAINT")

		err := installer.InstallCniConf(cniConfName, cniEtcDir, cniNetConf, cniNetConfFile)
		if err != nil {
			log.ErrorExit(err)
		}

		err = installer.InstallCoil(coilPath, cniBinDir)
		if err != nil {
			log.ErrorExit(err)
		}

		err = installer.EnableIPForwarding()
		if err != nil {
			log.ErrorExit(err)
		}

		err = installer.RemoveBootTaintFromNode(coilNodeName, coilBootTaint)
		if err != nil {
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
	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	viper.BindEnv("CNI_CONF_NAME")
	viper.BindEnv("CNI_ETC_DIR")
	viper.BindEnv("CNI_BIN_DIR")
	viper.BindEnv("COIL_PATH")
	viper.BindEnv("CNI_NETCONF_FILE")
	viper.BindEnv("CNI_NETCONF")
	viper.BindEnv("COIL_NODE_NAME")
	viper.BindEnv("COIL_BOOT_TAINT")

	viper.SetDefault("CNI_CONF_NAME", defaultCniConfName)
	viper.SetDefault("CNI_ETC_DIR", defaultCniEtcDir)
	viper.SetDefault("CNI_BIN_DIR", defaultCniBinDir)
	viper.SetDefault("COIL_PATH", defaultCoilPath)
	viper.SetDefault("COIL_BOOT_TAINT", defaultBootTaint)
}
