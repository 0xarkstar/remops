package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// envVarPattern matches ${VAR} or ${VAR:-default} patterns.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads and parses the remops config from the first found path.
func Load() (*Config, error) {
	paths := DefaultConfigPaths()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return parse(data)
	}
	return nil, fmt.Errorf("config file not found; searched: %s\n\nRun 'remops init' to create a config file", strings.Join(paths, ", "))
}

// LoadFrom reads and parses a specific config file.
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config %s: %w", path, err)
	}
	return parse(data)
}

func parse(data []byte) (*Config, error) {
	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// expandEnvVars replaces ${VAR} and ${VAR:-default} with environment variable values.
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-1] // strip ${ and }

		name, defaultVal, hasDefault := strings.Cut(inner, ":-")
		val := os.Getenv(name)
		if val == "" && hasDefault {
			return defaultVal
		}
		if val == "" {
			return match // leave unexpanded if no value and no default
		}
		return val
	})
}

func validate(cfg *Config) error {
	if cfg.Version != 1 {
		return fmt.Errorf("unsupported config version: %d (expected 1)", cfg.Version)
	}

	if len(cfg.Hosts) == 0 {
		return fmt.Errorf("at least one host must be defined")
	}

	for name, host := range cfg.Hosts {
		if host.Address == "" {
			return fmt.Errorf("host %q: address is required", name)
		}
	}

	for name, svc := range cfg.Services {
		if svc.Host == "" {
			return fmt.Errorf("service %q: host is required", name)
		}
		if _, ok := cfg.Hosts[svc.Host]; !ok {
			return fmt.Errorf("service %q: references unknown host %q", name, svc.Host)
		}
		if svc.Container == "" {
			return fmt.Errorf("service %q: container is required", name)
		}
	}

	for name, profile := range cfg.Profiles {
		switch profile.Level {
		case "viewer", "operator", "admin":
			// valid
		default:
			return fmt.Errorf("profile %q: invalid level %q (must be viewer, operator, or admin)", name, profile.Level)
		}
	}

	if cfg.Approval != nil && cfg.Approval.Method == "telegram" {
		if cfg.Approval.BotToken == "" {
			return fmt.Errorf("approval: telegram method requires non-empty bot_token")
		}
		if cfg.Approval.ChatID == "" {
			return fmt.Errorf("approval: telegram method requires non-empty chat_id")
		}
	}

	for name, svc := range cfg.Services {
		if svc.DB != nil {
			switch svc.DB.Engine {
			case "postgresql", "postgres", "mysql":
				// valid
			case "":
				return fmt.Errorf("service %q: db.engine is required", name)
			default:
				return fmt.Errorf("service %q: unsupported db.engine %q (must be postgresql or mysql)", name, svc.DB.Engine)
			}
			if svc.DB.User == "" {
				return fmt.Errorf("service %q: db.user is required", name)
			}
			if svc.DB.Database == "" {
				return fmt.Errorf("service %q: db.database is required", name)
			}
		}
	}

	return nil
}

// HostsByTag returns host names matching any of the given tags.
func (c *Config) HostsByTag(tags ...string) []string {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}

	var result []string
	for name, host := range c.Hosts {
		for _, t := range host.Tags {
			if tagSet[t] {
				result = append(result, name)
				break
			}
		}
	}
	return result
}

// ServicesByTag returns service names matching any of the given tags.
func (c *Config) ServicesByTag(tags ...string) []string {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}

	var result []string
	for name, svc := range c.Services {
		for _, t := range svc.Tags {
			if tagSet[t] {
				result = append(result, name)
				break
			}
		}
	}
	return result
}

// AllHostNames returns all host names sorted.
func (c *Config) AllHostNames() []string {
	names := make([]string, 0, len(c.Hosts))
	for name := range c.Hosts {
		names = append(names, name)
	}
	return names
}

// AllServiceNames returns all service names.
func (c *Config) AllServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for name := range c.Services {
		names = append(names, name)
	}
	return names
}
