package sub

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	v2 "github.com/cybozu-go/coil/v2"
)

const (
	defaultCniConfName = "10-coil.conflist"
	defaultCniEtcDir   = "/host/etc/cni/net.d"
	defaultCniBinDir   = "/host/opt/cni/bin"
	defaultCoilPath    = "/usr/local/coil/coil"
)

var rootCmd = &cobra.Command{
	Use:     "coil-installer",
	Short:   "install coil CNI binary and configuration files",
	Long:    `coil-installer setup coil on each node by installing CNI binary and config files.`,
	Version: v2.Version(),
	RunE: func(cmd *cobra.Command, _ []string) error {
		cniConfName := viper.GetString("CNI_CONF_NAME")
		cniEtcDir := viper.GetString("CNI_ETC_DIR")
		cniBinDir := viper.GetString("CNI_BIN_DIR")
		coilPath := viper.GetString("COIL_PATH")
		cniNetConf := viper.GetString("CNI_NETCONF")
		cniNetConfFile := viper.GetString("CNI_NETCONF_FILE")

		err := installCniConf(cniConfName, cniEtcDir, cniNetConf, cniNetConfFile)
		if err != nil {
			return err
		}

		err = installCoil(coilPath, cniBinDir)
		if err != nil {
			return err
		}

		// Because kubelet scans /etc/cni/net.d for CNI config files every 5 seconds,
		// the installer need to sleep at least 5 seconds before finish.
		// ref: https://github.com/kubernetes/kubernetes/blob/3d9c6eb9e6e1759683d2c6cda726aad8a0e85604/pkg/kubelet/kubelet.go#L1416
		time.Sleep(6 * time.Second)
		return nil
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
	viper.BindEnv("COIL_BOOT_TAINT")

	viper.SetDefault("CNI_CONF_NAME", defaultCniConfName)
	viper.SetDefault("CNI_ETC_DIR", defaultCniEtcDir)
	viper.SetDefault("CNI_BIN_DIR", defaultCniBinDir)
	viper.SetDefault("COIL_PATH", defaultCoilPath)
}
