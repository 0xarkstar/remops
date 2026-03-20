package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugins",
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available plugins",
	Run: func(cmd *cobra.Command, args []string) {
		plugins := pluginRegistry.All()
		if len(plugins) == 0 {
			fmt.Println("No plugins registered.")
			return
		}
		for _, p := range plugins {
			enabled := "disabled"
			if cfg != nil {
				for _, name := range cfg.Plugins {
					if name == p.Name() {
						enabled = "enabled"
						break
					}
				}
			}
			fmt.Printf("%-15s v%-8s %s [%s]\n", p.Name(), p.Version(), p.Description(), enabled)
		}
	},
}

func init() {
	pluginCmd.AddCommand(pluginListCmd)
	rootCmd.AddCommand(pluginCmd)
}
