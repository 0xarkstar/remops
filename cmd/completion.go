package cmd

import (
	"os"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion scripts for remops.

To load completions:

  Bash:
    source <(remops completion bash)
    # Persist: remops completion bash > /etc/bash_completion.d/remops

  Zsh:
    source <(remops completion zsh)
    # Persist: remops completion zsh > "${fpath[1]}/_remops"

  Fish:
    remops completion fish | source
    # Persist: remops completion fish > ~/.config/fish/completions/remops.fish

  PowerShell:
    remops completion powershell | Out-String | Invoke-Expression
`,
	ValidArgs:         []string{"bash", "zsh", "fish", "powershell"},
	Args:              cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableAutoGenTag: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

// registerCompletions is called from root.go init after flags are defined.
func registerCompletions() {
	_ = rootCmd.RegisterFlagCompletionFunc("host", completeHostNames)
	_ = rootCmd.RegisterFlagCompletionFunc("profile", completeProfileNames)
	_ = rootCmd.RegisterFlagCompletionFunc("tag", completeTags)
}

// loadConfigSilent loads config without returning errors — safe for completions.
func loadConfigSilent() *config.Config {
	cfg, _ := config.Load()
	return cfg
}

func completeHostNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := loadConfigSilent()
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, n := range cfg.AllHostNames() {
		if strings.HasPrefix(n, toComplete) {
			names = append(names, n)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completeProfileNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := loadConfigSilent()
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for name := range cfg.Profiles {
		if strings.HasPrefix(name, toComplete) {
			names = append(names, name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completeTags(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := loadConfigSilent()
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := make(map[string]struct{})
	for _, host := range cfg.Hosts {
		for _, t := range host.Tags {
			seen[t] = struct{}{}
		}
	}
	for _, svc := range cfg.Services {
		for _, t := range svc.Tags {
			seen[t] = struct{}{}
		}
	}
	var tags []string
	for t := range seen {
		if strings.HasPrefix(t, toComplete) {
			tags = append(tags, t)
		}
	}
	return tags, cobra.ShellCompDirectiveNoFileComp
}

// completeServiceNames is a reusable completion function for service name flags/args.
func completeServiceNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := loadConfigSilent()
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, n := range cfg.AllServiceNames() {
		if strings.HasPrefix(n, toComplete) {
			names = append(names, n)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeStackNames is a reusable completion function for stack name args.
func completeStackNames(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg := loadConfigSilent()
	if cfg == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for _, n := range cfg.AllStackNames() {
		if strings.HasPrefix(n, toComplete) {
			names = append(names, n)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeSinceDurations suggests common duration values for --since flags.
func completeSinceDurations(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	suggestions := []string{"1m", "5m", "15m", "1h", "6h", "24h", "7d"}
	var matches []string
	for _, s := range suggestions {
		if strings.HasPrefix(s, toComplete) {
			matches = append(matches, s)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}
