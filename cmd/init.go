package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/0xarkstar/remops/internal/config"
	sshconfig "github.com/kevinburke/ssh_config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	flagInitOutput string
	flagInitMCP    bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new remops.yaml configuration file",
	Long: `Interactively create a remops.yaml configuration file.

Walks you through adding hosts and writes the config to disk.
Run 'remops doctor' after init to verify SSH connectivity.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		outputPath := flagInitOutput
		if outputPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			outputPath = filepath.Join(home, ".config", "remops", "remops.yaml")
		}

		scanner := bufio.NewScanner(os.Stdin)

		// Check for existing config.
		if _, err := os.Stat(outputPath); err == nil {
			fmt.Printf("Config already exists at %s. Overwrite? [y/N]: ", outputPath)
			scanner.Scan()
			answer := strings.TrimSpace(scanner.Text())
			if !strings.EqualFold(answer, "y") {
				fmt.Println("Aborted.")
				return nil
			}
		}

		// Determine default SSH user.
		defaultUser := "root"
		if u, err := user.Current(); err == nil && u.Username != "" {
			defaultUser = u.Username
		}

		hosts := make(map[string]config.Host)

		// Try to discover hosts from SSH config.
		sshHosts := discoverSSHConfigHosts()
		if len(sshHosts) > 0 {
			fmt.Println("\nFound hosts in ~/.ssh/config:")
			for i, h := range sshHosts {
				fmt.Printf("  %d. %s (%s, user: %s)\n", i+1, h.Name, h.Address, h.User)
			}
			fmt.Print("\nImport these hosts? [Y/n]: ")
			scanner.Scan()
			answer := strings.TrimSpace(scanner.Text())
			if answer == "" || strings.EqualFold(answer, "y") {
				for _, h := range sshHosts {
					host := config.Host{
						Address: h.Address,
						User:    h.User,
					}
					if h.Port != 22 {
						host.Port = h.Port
					}
					if h.Key != "" {
						host.Key = h.Key
					}
					hosts[h.Name] = host
				}
				fmt.Printf("Imported %d host(s).\n", len(sshHosts))
			}
		}

		for {
			fmt.Println()
			name := prompt(scanner, "Host name (e.g. prod, dev)", "prod")
			address := ""
			for address == "" {
				address = prompt(scanner, "Host address (IP or hostname)", "")
				if address == "" {
					fmt.Println("  Address is required.")
				}
			}
			sshUser := prompt(scanner, "SSH user", defaultUser)
			portStr := prompt(scanner, "SSH port", "22")
			port, err := strconv.Atoi(portStr)
			if err != nil || port <= 0 {
				port = 22
			}
			description := prompt(scanner, "Description (optional)", "")

			h := config.Host{
				Address: address,
				User:    sshUser,
			}
			if port != 22 {
				h.Port = port
			}
			if description != "" {
				h.Description = description
			}
			hosts[name] = h

			fmt.Print("\nAdd another host? [y/N]: ")
			scanner.Scan()
			again := strings.TrimSpace(scanner.Text())
			if !strings.EqualFold(again, "y") {
				break
			}
		}

		cfg := buildDefaultConfig(hosts)

		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		if err := os.WriteFile(outputPath, data, 0o600); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		fmt.Printf("\nConfig written to %s\n", outputPath)
		fmt.Println("Run 'remops doctor' to verify connectivity.")

		if flagInitMCP {
			if err := setupMCPConfig(); err != nil {
				return fmt.Errorf("mcp setup: %w", err)
			}
		}

		return nil
	},
}

type sshHostEntry struct {
	Name    string
	Address string
	User    string
	Port    int
	Key     string
}

func discoverSSHConfigHosts() []sshHostEntry {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	defer f.Close()

	cfg, err := sshconfig.Decode(f)
	if err != nil {
		return nil
	}

	var hosts []sshHostEntry
	for _, host := range cfg.Hosts {
		if len(host.Patterns) == 0 {
			continue
		}
		name := host.Patterns[0].String()
		if name == "*" || name == "" || strings.ContainsAny(name, "*?") {
			continue
		}

		hostName, _ := cfg.Get(name, "HostName")
		if hostName == "" {
			continue
		}

		sshUser, _ := cfg.Get(name, "User")
		portStr, _ := cfg.Get(name, "Port")
		identityFile, _ := cfg.Get(name, "IdentityFile")

		port := 22
		if portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				port = p
			}
		}

		if strings.HasPrefix(identityFile, "~/") {
			identityFile = filepath.Join(home, identityFile[2:])
		}

		hosts = append(hosts, sshHostEntry{
			Name:    name,
			Address: hostName,
			User:    sshUser,
			Port:    port,
			Key:     identityFile,
		})
	}
	return hosts
}

func buildDefaultConfig(hosts map[string]config.Host) *config.Config {
	return &config.Config{
		Version: 1,
		Hosts:   hosts,
		Profiles: map[string]config.Profile{
			"viewer": {
				Level: "viewer",
			},
			"operator": {
				Level:    "operator",
				Approval: "telegram",
			},
			"admin": {
				Level: "admin",
			},
		},
	}
}

func prompt(scanner *bufio.Scanner, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if val == "" {
		return defaultVal
	}
	return val
}

func init() {
	initCmd.Flags().StringVar(&flagInitOutput, "output", "", "Path to write the config file (default: ~/.config/remops/remops.yaml)")
	initCmd.Flags().BoolVar(&flagInitMCP, "mcp", false, "Auto-configure Claude Code MCP integration after init")
	rootCmd.AddCommand(initCmd)
}
