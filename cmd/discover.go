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

// discoveredStack holds a Docker Compose project found on a host that is not yet in config.
type discoveredStack struct {
	Host string `json:"host"`
	Path string `json:"path"`
	Name string `json:"name"` // derived from directory name
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

var flagDiscoverStacks bool

func init() {
	rootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().BoolVar(&flagDiscoverStacks, "stacks", false, "Also discover Docker Compose projects")
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

	// Discover Docker Compose stacks if requested.
	var allNewStacks []discoveredStack
	if flagDiscoverStacks {
		existingStacks := cfg.Stacks
		if existingStacks == nil {
			existingStacks = map[string]config.Stack{}
		}
		allNewStacks = discoverStacks(cmd.Context(), tr, hosts, existingStacks)
		printStackDiscovery(cmd.OutOrStdout(), allNewStacks, existingStacks, hosts)
	}

	if len(allNew) == 0 && len(allNewStacks) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "All services are already up to date.")
		return nil
	}

	if flagFormat == "json" {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(allNew)
	}

	if len(allNew) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Found %d new container(s). Add to config? [Y/n]: ", len(allNew))
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("failed to read input: %w", scanner.Err())
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer != "" && !strings.EqualFold(answer, "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}

		if err := appendServicesToConfig(allNew); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Added %d service(s) to config.\n", len(allNew))
	}

	if len(allNewStacks) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Found %d new stack(s). Add to config? [Y/n]: ", len(allNewStacks))
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("failed to read input: %w", scanner.Err())
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer != "" && !strings.EqualFold(answer, "y") {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}

		if err := appendStacksToConfig(allNewStacks); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Added %d stack(s) to config.\n", len(allNewStacks))
	}

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
			defer mu.Unlock()
			results = append(results, r)
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

	services, ok := raw["services"].(map[string]interface{})
	if !ok || services == nil {
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

// findCmd is the shell command used to locate Docker Compose files on a host.
const findComposeCmd = `find /home -name "docker-compose.yml" -o -name "docker-compose.yaml" -o -name "compose.yml" -o -name "compose.yaml" 2>/dev/null`

// discoverStacks queries each host for Docker Compose projects not yet in config.
func discoverStacks(ctx context.Context, tr transport.Transport, hosts []string, existingStacks map[string]config.Stack) []discoveredStack {
	var results []discoveredStack
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, h := range hosts {
		wg.Add(1)
		go func(hostName string) {
			defer wg.Done()

			res, err := tr.Exec(ctx, hostName, findComposeCmd)
			if err != nil || res.ExitCode != 0 {
				return
			}

			for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Extract the directory containing the compose file.
				idx := strings.LastIndex(line, "/")
				if idx < 0 {
					continue
				}
				dir := line[:idx]
				if dir == "" {
					dir = "/"
				}
				nameIdx := strings.LastIndex(dir, "/")
				rawName := dir[nameIdx+1:]
				name := sanitizeName(rawName)
				if name == "" {
					continue
				}

				// Check if this host+path is already in config.
				known := false
				for _, s := range existingStacks {
					if s.Host == hostName && s.Path == dir {
						known = true
						break
					}
				}
				if known {
					continue
				}

				mu.Lock()
				results = append(results, discoveredStack{Host: hostName, Path: dir, Name: name})
				mu.Unlock()
			}
		}(h)
	}

	wg.Wait()
	return results
}

// printStackDiscovery writes the per-host stacks discovery report to w.
func printStackDiscovery(w interface{ Write([]byte) (int, error) }, stacks []discoveredStack, existingStacks map[string]config.Stack, hosts []string) {
	// Group new stacks by host.
	byHost := make(map[string][]discoveredStack)
	for _, s := range stacks {
		byHost[s.Host] = append(byHost[s.Host], s)
	}

	for _, h := range hosts {
		hostStacks, hasNew := byHost[h]
		// Collect known stacks for this host.
		var known []config.Stack
		for _, s := range existingStacks {
			if s.Host == h {
				known = append(known, s)
			}
		}
		if !hasNew && len(known) == 0 {
			continue
		}
		fmt.Fprintf(w, "Compose stacks on %s:\n", h)
		for _, s := range hostStacks {
			fmt.Fprintf(w, "  + %-20s (%s)\n", s.Name, s.Path)
		}
		for _, s := range known {
			fmt.Fprintf(w, "  \u2713 %-20s (already in config)\n", s.Path)
		}
	}
}

// appendStacksToConfig reads the existing config file, merges new stacks, and writes it back.
func appendStacksToConfig(stacks []discoveredStack) error {
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

	stacksMap, ok := raw["stacks"].(map[string]interface{})
	if !ok || stacksMap == nil {
		stacksMap = make(map[string]interface{})
	}

	for _, s := range stacks {
		key := uniqueServiceKey(stacksMap, s.Name)
		stacksMap[key] = map[string]interface{}{
			"host": s.Host,
			"path": s.Path,
		}
	}
	raw["stacks"] = stacksMap

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("cannot marshal updated config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0o600); err != nil {
		return fmt.Errorf("cannot write config %s: %w", configPath, err)
	}

	return nil
}
