package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dits",
	Short: "Distributed Issue Tracking System",
	Long:  "DITS is a local-first, event-sourced distributed issue tracker.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(workCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(remoteCmd)
	rootCmd.AddCommand(metaCmd)
	rootCmd.AddCommand(identityCmd)
}
