package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/docker"
	"github.com/0xarkstar/remops/internal/transport"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// discoveredContainer holds a container found on a host that is not yet in config.
type discoveredContainer struct {
	Host      string `json:"host"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	Status    string `json:"status"`
	ServiceID string `json:"service_id"` // sanitized name for config key
}

// discoverHostResult holds scan results for a single host.
type discoverHostResult struct {
	HostName  string
	New       []discoveredContainer
	Known     []string // container names already in config
	Err       error
}

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover running containers across hosts and add them to config",
	Long: `Scan configured hosts for running Docker containers not yet in your config.

Discovered containers can be added as services with a single confirmation.
Use --host or --tag to limit which hosts to scan.`,
	RunE: runDiscover,
}

func init() {
	rootCmd.AddCommand(discoverCmd)
}

func runDiscover(cmd *cobra.Command, args []string) error {
	hosts := resolveHosts()
	if len(hosts) == 0 {
		return fmt.Errorf("no hosts to scan")
	}

	tr := transport.NewSSHTransport(cfg)
	defer tr.Close()

	dc := docker.NewDockerClient(tr)

	fmt.Fprintln(cmd.OutOrStdout(), "Scanning hosts...")
	fmt.Fprintln(cmd.OutOrStdout())

	results := scanHosts(cmd.Context(), dc, cfg, hosts)

	// Sort results by host name for deterministic output.
	sort.Slice(results, func(i, j int) bool {
		return results[i].HostName < results[j].HostName
	})

	var allNew []discoveredContainer

	for _, r := range results {
		hostCfg := cfg.Hosts[r.HostName]
		fmt.Fprintf(cmd.OutOrStdout(), "%s (%s):\n", r.HostName, hostCfg.Address)

		if r.Err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  (error: %v)\n", r.Err)
			fmt.Fprintln(cmd.OutOrStdout())
			continue
		}

		if len(r.New) == 0 && len(r.Known) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "  (no containers running)")
			fmt.Fprintln(cmd.OutOrStdout())
			continue
		}

		for _, c := range r.New {
			fmt.Fprintf(cmd.OutOrStdout(), "  + %-22s (%s)  %s\n", c.Name, c.Status, c.Image)
			allNew = append(allNew, c)
		}
		for _, name := range r.Known {
			fmt.Fprintf(cmd.OutOrStdout(), "  \u2713 %-22s (already in config)\n", name)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if len(allNew) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "All services are already up to date.")
		return nil
	}

	if flagFormat == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(allNew)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Found %d new container(s). Add to config? [Y/n]: ", len(allNew))
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())
	if answer != "" && !strings.EqualFold(answer, "y") {
		fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
		return nil
	}

	if err := appendServicesToConfig(allNew); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added %d service(s) to config.\n", len(allNew))
	return nil
}

// scanHosts queries all given hosts in parallel and returns per-host results.
func scanHosts(ctx context.Context, dc *docker.DockerClient, cfg *config.Config, hosts []string) []discoverHostResult {
	results := make([]discoverHostResult, 0, len(hosts))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, h := range hosts {
		wg.Add(1)
		go func(hostName string) {
			defer wg.Done()

			hostCfg, ok := cfg.Hosts[hostName]
			timeout := 10 * time.Second
			if ok {
				timeout = hostCfg.EffectiveTimeout()
			}
			hostCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			containers, err := dc.ListContainers(hostCtx, hostName)
			r := discoverHostResult{HostName: hostName}
			if err != nil {
				r.Err = err
			} else {
				r.New, r.Known = classifyContainers(containers, cfg, hostName)
			}

			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(h)
	}

	wg.Wait()
	return results
}

// classifyContainers splits containers into new (not in config) and known (already in config).
func classifyContainers(containers []docker.ContainerInfo, cfg *config.Config, hostName string) (newContainers []discoveredContainer, knownNames []string) {
	for _, c := range containers {
		if isKnownContainer(cfg, hostName, c.Name) {
			knownNames = append(knownNames, c.Name)
		} else {
			newContainers = append(newContainers, discoveredContainer{
				Host:      hostName,
				Name:      c.Name,
				Image:     c.Image,
				Status:    c.Status,
				ServiceID: sanitizeName(c.Name),
			})
		}
	}
	return newContainers, knownNames
}

// isKnownContainer returns true if any existing service maps to this container on this host.
func isKnownContainer(cfg *config.Config, hostName, containerName string) bool {
	for _, svc := range cfg.Services {
		if svc.Host == hostName && svc.Container == containerName {
			return true
		}
	}
	return false
}

// sanitizeName replaces dots and special characters with hyphens to produce a valid service key.
var nonAlphanumDash = regexp.MustCompile(`[^a-zA-Z0-9\-_]`)

func sanitizeName(name string) string {
	s := strings.ReplaceAll(name, ".", "-")
	s = nonAlphanumDash.ReplaceAllString(s, "-")
	return strings.ToLower(s)
}

// appendServicesToConfig reads the existing config file, merges new services, and writes it back.
func appendServicesToConfig(containers []discoveredContainer) error {
	paths := config.DefaultConfigPaths()
	var configPath string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}
	if configPath == "" {
		return fmt.Errorf("config file not found; cannot write back")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("cannot read config %s: %w", configPath, err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("cannot parse config: %w", err)
	}

	services, _ := raw["services"].(map[string]interface{})
	if services == nil {
		services = make(map[string]interface{})
	}

	for _, c := range containers {
		key := uniqueServiceKey(services, c.ServiceID)
		services[key] = map[string]interface{}{
			"host":      c.Host,
			"container": c.Name,
		}
	}
	raw["services"] = services

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("cannot marshal updated config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0o600); err != nil {
		return fmt.Errorf("cannot write config %s: %w", configPath, err)
	}

	return nil
}

// uniqueServiceKey returns key if not taken, otherwise appends -2, -3, ... until unique.
func uniqueServiceKey(services map[string]interface{}, base string) string {
	if _, exists := services[base]; !exists {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, exists := services[candidate]; !exists {
			return candidate
		}
	}
}
