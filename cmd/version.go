package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print remops version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("remops %s (%s) built %s\n", buildVersion, buildCommit, buildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
