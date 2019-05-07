package cmd

import (
	"github.com/spf13/cobra"
)

// addressCmd represents the address command
var addressCmd = &cobra.Command{
	Use:   "address",
	Short: "address subcommand",
	Long: `address subcommand is the parent of commands that edit or
show address assignments in etcd.`,
}

func init() {
	rootCmd.AddCommand(addressCmd)
}
