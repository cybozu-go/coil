package sub

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "coil-migrator",
	Short: "helper to migrate old coil data to new one",
	Long:  `coil-migrator helps live migration from Coil v1 to v2.`,
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
	pf := rootCmd.PersistentFlags()
	// this is to set controller-runtime/pkg/client/config's flag
	// https://github.com/kubernetes-sigs/controller-runtime/blob/c000ea850121b53dd769f624f3bfa74a4fcf100f/pkg/client/config/config.go#L39
	pf.AddGoFlag(flag.CommandLine.Lookup("kubeconfig"))
}
