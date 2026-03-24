package config

import "time"

// Config is the top-level remops configuration.
type Config struct {
	Version  int                `yaml:"version"`
	Hosts    map[string]Host    `yaml:"hosts"`
	Services map[string]Service `yaml:"services"`
	Profiles map[string]Profile `yaml:"profiles"`
	Presets  map[string]string  `yaml:"presets,omitempty"`
	Plugins  []string           `yaml:"plugins,omitempty"`
	Approval *ApprovalConfig    `yaml:"approval,omitempty"`
	API      *APIConfig         `yaml:"api,omitempty"`
}

// Host defines a remote host connection.
type Host struct {
	Address     string         `yaml:"address"`
	User        string         `yaml:"user,omitempty"`
	Port        int            `yaml:"port,omitempty"`
	Key         string         `yaml:"key,omitempty"`
	SSHConfig   string         `yaml:"ssh_config,omitempty"`
	ProxyJump   string         `yaml:"proxy_jump,omitempty"`
	Timeout     string         `yaml:"timeout,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Tags        []string       `yaml:"tags,omitempty"`
	Meta        map[string]any `yaml:"meta,omitempty"`
}

// EffectivePort returns the SSH port, defaulting to 22.
func (h Host) EffectivePort() int {
	if h.Port == 0 {
		return 22
	}
	return h.Port
}

// EffectiveUser returns the SSH user, defaulting to current user.
func (h Host) EffectiveUser() string {
	if h.User == "" {
		return "root"
	}
	return h.User
}

// EffectiveTimeout returns the parsed timeout duration, defaulting to 10s.
func (h Host) EffectiveTimeout() time.Duration {
	if h.Timeout == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(h.Timeout)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// Service maps a logical name to a container on a host.
type Service struct {
	Host      string    `yaml:"host"`
	Container string    `yaml:"container"`
	Tags      []string  `yaml:"tags,omitempty"`
	DB        *DBConfig `yaml:"db,omitempty"`
}

// DBConfig defines database connection settings for a service.
type DBConfig struct {
	Engine   string `yaml:"engine"`             // "postgresql", "mysql"
	User     string `yaml:"user"`
	Password string `yaml:"password,omitempty"` // supports ${ENV_VAR}
	Database string `yaml:"database"`
	Port     int    `yaml:"port,omitempty"`
}

// Profile defines permission levels for CLI usage.
type Profile struct {
	Level         string `yaml:"level"`
	RequireDryRun bool   `yaml:"require_dry_run,omitempty"`
	Approval      string `yaml:"approval,omitempty"`
}

// PermissionLevel represents the access level.
type PermissionLevel int

const (
	LevelViewer   PermissionLevel = iota // Read-only operations
	LevelOperator                        // Write operations with approval
	LevelAdmin                           // Full access
)

// ParseLevel converts a string to a PermissionLevel.
func ParseLevel(s string) PermissionLevel {
	switch s {
	case "viewer":
		return LevelViewer
	case "operator":
		return LevelOperator
	case "admin":
		return LevelAdmin
	default:
		return LevelViewer
	}
}

// String returns the string representation of a PermissionLevel.
func (p PermissionLevel) String() string {
	switch p {
	case LevelViewer:
		return "viewer"
	case LevelOperator:
		return "operator"
	case LevelAdmin:
		return "admin"
	default:
		return "viewer"
	}
}

// ApprovalConfig defines out-of-band approval settings.
type ApprovalConfig struct {
	Method    string           `yaml:"method"`
	BotToken  string           `yaml:"bot_token"`
	ChatID    string           `yaml:"chat_id"`
	Timeout   string           `yaml:"timeout,omitempty"`
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
}

// EffectiveTimeout returns the approval timeout, defaulting to 5m.
func (a ApprovalConfig) EffectiveTimeout() time.Duration {
	if a.Timeout == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(a.Timeout)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

// APIConfig defines HTTP API server settings.
type APIConfig struct {
	Listen string `yaml:"listen"`
	APIKey string `yaml:"api_key"`
}

// EffectiveListen returns the listen address, defaulting to ":9090".
func (a APIConfig) EffectiveListen() string {
	if a.Listen == "" {
		return ":9090"
	}
	return a.Listen
}

// RateLimitConfig defines write rate limiting.
type RateLimitConfig struct {
	MaxWritesPerHostPerHour int `yaml:"max_writes_per_host_per_hour"`
}

// EffectiveMaxWrites returns the max writes per host per hour, defaulting to 5.
func (r RateLimitConfig) EffectiveMaxWrites() int {
	if r.MaxWritesPerHostPerHour <= 0 {
		return 5
	}
	return r.MaxWritesPerHostPerHour
}
