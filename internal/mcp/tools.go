package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xarkstar/remops/internal/config"
	"github.com/0xarkstar/remops/internal/security"
)

// ToolDef describes a single MCP tool for the tools/list response.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// mcpContent wraps text in the MCP content envelope format.
func mcpContent(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": security.SanitizeOutput(text)},
		},
	}
}

// registerTools populates s.tools and s.defs with all available tools.
func registerTools(s *Server) {
	type reg struct {
		def     ToolDef
		handler ToolHandler
	}

	registrations := []reg{
		{
			def: ToolDef{
				Name:        "remops_status",
				Description: "List running Docker containers on one or more hosts.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host": map[string]any{"type": "string", "description": "Target host name (optional)"},
						"tag":  map[string]any{"type": "string", "description": "Filter hosts by tag (optional)"},
					},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := security.CheckPermission(s.profileLevel, config.LevelViewer); err != nil {
					return nil, err
				}
				var p struct {
					Host string `json:"host"`
					Tag  string `json:"tag"`
				}
				if err := json.Unmarshal(raw, &p); err != nil {
					return nil, fmt.Errorf("invalid parameters: %w", err)
				}

				if p.Host != "" {
					if err := security.ValidateHostName(p.Host); err != nil {
						return nil, err
					}
				}

				hosts := resolveHosts(s, p.Host, p.Tag)
				if len(hosts) == 0 {
					return mcpContent("no hosts matched"), nil
				}

				var sb strings.Builder
				for _, h := range hosts {
					res, err := s.transport.Exec(ctx, h, `docker ps --format '{{json .}}'`)
					if err != nil {
						fmt.Fprintf(&sb, "# %s\nerror: %v\n", h, err)
						continue
					}
					fmt.Fprintf(&sb, "# %s\n%s\n", h, res.Stdout)
				}
				return mcpContent(sb.String()), nil
			},
		},
		{
			def: ToolDef{
				Name:        "remops_service_logs",
				Description: "Fetch logs for a named service.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"service": map[string]any{"type": "string", "description": "Service name"},
						"tail":    map[string]any{"type": "integer", "description": "Number of lines to tail"},
						"since":   map[string]any{"type": "string", "description": "Show logs since timestamp or duration (e.g. 1h)"},
					},
					"required": []string{"service"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := security.CheckPermission(s.profileLevel, config.LevelViewer); err != nil {
					return nil, err
				}
				var p struct {
					Service string `json:"service"`
					Tail    int    `json:"tail"`
					Since   string `json:"since"`
				}
				if err := json.Unmarshal(raw, &p); err != nil || p.Service == "" {
					return nil, fmt.Errorf("service is required")
				}
				if err := security.ValidateServiceName(p.Service); err != nil {
					return nil, err
				}
				if p.Since != "" {
					if err := security.DetectShellInjection(p.Since); err != nil {
						return nil, fmt.Errorf("invalid since value: %w", err)
					}
				}

				svc, ok := s.config.Services[p.Service]
				if !ok {
					return nil, fmt.Errorf("unknown service: %s", p.Service)
				}

				cmd := "docker logs " + svc.Container
				if p.Tail > 0 {
					cmd += fmt.Sprintf(" --tail %d", p.Tail)
				}
				if p.Since != "" {
					cmd += " --since " + p.Since
				}

				res, err := s.transport.Exec(ctx, svc.Host, cmd)
				if err != nil {
					return nil, err
				}
				combined := res.Stdout
				if res.Stderr != "" {
					combined += res.Stderr
				}
				return mcpContent(combined), nil
			},
		},
		{
			def: ToolDef{
				Name:        "remops_service_restart",
				Description: "Restart a named service container.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"service": map[string]any{"type": "string", "description": "Service name"},
						"confirm": map[string]any{"type": "boolean", "description": "Must be true to execute"},
					},
					"required": []string{"service", "confirm"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				return serviceLifecycle(ctx, s, raw, "restart")
			},
		},
		{
			def: ToolDef{
				Name:        "remops_service_stop",
				Description: "Stop a named service container.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"service": map[string]any{"type": "string", "description": "Service name"},
						"confirm": map[string]any{"type": "boolean", "description": "Must be true to execute"},
					},
					"required": []string{"service", "confirm"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				return serviceLifecycle(ctx, s, raw, "stop")
			},
		},
		{
			def: ToolDef{
				Name:        "remops_service_start",
				Description: "Start a named service container.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"service": map[string]any{"type": "string", "description": "Service name"},
						"confirm": map[string]any{"type": "boolean", "description": "Must be true to execute"},
					},
					"required": []string{"service", "confirm"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				return serviceLifecycle(ctx, s, raw, "start")
			},
		},
		{
			def: ToolDef{
				Name:        "remops_host_info",
				Description: "Get system and Docker version info for a host.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host": map[string]any{"type": "string", "description": "Host name"},
					},
					"required": []string{"host"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := security.CheckPermission(s.profileLevel, config.LevelViewer); err != nil {
					return nil, err
				}
				var p struct {
					Host string `json:"host"`
				}
				if err := json.Unmarshal(raw, &p); err != nil || p.Host == "" {
					return nil, fmt.Errorf("host is required")
				}
				if err := security.ValidateHostName(p.Host); err != nil {
					return nil, err
				}

				var sb strings.Builder
				for _, cmd := range []string{"uname -a", "docker version --format '{{json .}}'"} {
					res, err := s.transport.Exec(ctx, p.Host, cmd)
					if err != nil {
						fmt.Fprintf(&sb, "$ %s\nerror: %v\n", cmd, err)
						continue
					}
					fmt.Fprintf(&sb, "$ %s\n%s\n", cmd, res.Stdout)
				}
				return mcpContent(sb.String()), nil
			},
		},
		{
			def: ToolDef{
				Name:        "remops_host_disk",
				Description: "Show disk usage for a host using df.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host": map[string]any{"type": "string", "description": "Host name"},
					},
					"required": []string{"host"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := security.CheckPermission(s.profileLevel, config.LevelViewer); err != nil {
					return nil, err
				}
				var p struct {
					Host string `json:"host"`
				}
				if err := json.Unmarshal(raw, &p); err != nil || p.Host == "" {
					return nil, fmt.Errorf("host is required")
				}
				if err := security.ValidateHostName(p.Host); err != nil {
					return nil, err
				}
				res, err := s.transport.Exec(ctx, p.Host, "df -h --output=target,size,used,avail,pcent")
				if err != nil {
					return nil, fmt.Errorf("disk usage on %s: %w", p.Host, err)
				}
				return mcpContent(res.Stdout), nil
			},
		},
		{
			def: ToolDef{
				Name:        "remops_host_exec",
				Description: "Execute an arbitrary command on a host. Requires admin permission and confirm=true.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host":    map[string]any{"type": "string", "description": "Host name"},
						"command": map[string]any{"type": "string", "description": "Command to execute"},
						"confirm": map[string]any{"type": "boolean", "description": "Must be true to execute"},
					},
					"required": []string{"host", "command", "confirm"},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := security.CheckPermission(s.profileLevel, config.LevelAdmin); err != nil {
					return nil, err
				}
				var p struct {
					Host    string `json:"host"`
					Command string `json:"command"`
					Confirm bool   `json:"confirm"`
				}
				if err := json.Unmarshal(raw, &p); err != nil || p.Host == "" || p.Command == "" {
					return nil, fmt.Errorf("host and command are required")
				}
				if err := security.ValidateHostName(p.Host); err != nil {
					return nil, err
				}
				if !p.Confirm {
					return nil, fmt.Errorf("confirm must be true to execute command")
				}
				if s.auditLogger != nil {
					_ = s.auditLogger.Log(security.AuditEntry{
						Command: p.Command,
						Host:    p.Host,
						Profile: s.profileLevel.String(),
						Result:  "exec",
					})
				}
				res, err := s.transport.Exec(ctx, p.Host, p.Command)
				if err != nil {
					return nil, fmt.Errorf("exec on %s: %w", p.Host, err)
				}
				text := res.Stdout
				if res.Stderr != "" {
					text += res.Stderr
				}
				return mcpContent(text), nil
			},
		},
		{
			def: ToolDef{
				Name:        "remops_doctor",
				Description: "Run health checks: ping all hosts and verify Docker is accessible.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
				if err := security.CheckPermission(s.profileLevel, config.LevelViewer); err != nil {
					return nil, err
				}
				var sb strings.Builder
				for name := range s.config.Hosts {
					pr, err := s.transport.Ping(ctx, name)
					if err != nil || !pr.Online {
						fmt.Fprintf(&sb, "%-20s OFFLINE\n", name)
						continue
					}
					fmt.Fprintf(&sb, "%-20s ONLINE  latency=%s\n", name, pr.Latency)

					res, err := s.transport.Exec(ctx, name, "docker info --format '{{.ServerVersion}}'")
					if err != nil {
						fmt.Fprintf(&sb, "  docker: error: %v\n", err)
					} else {
						fmt.Fprintf(&sb, "  docker: v%s\n", strings.TrimSpace(res.Stdout))
					}
				}
				return mcpContent(sb.String()), nil
			},
		},
	}

	registrations = append(registrations, reg{
		def: ToolDef{
			Name:        "remops_db_query",
			Description: "Run a SQL query on a service's database via docker exec.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"service": map[string]any{"type": "string", "description": "Service name"},
					"query":   map[string]any{"type": "string", "description": "SQL query to execute"},
				},
				"required": []string{"service", "query"},
			},
		},
		handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			if err := security.CheckPermission(s.profileLevel, config.LevelViewer); err != nil {
				return nil, err
			}
			var p struct {
				Service string `json:"service"`
				Query   string `json:"query"`
			}
			if err := json.Unmarshal(raw, &p); err != nil || p.Service == "" {
				return nil, fmt.Errorf("service is required")
			}
			if p.Query == "" {
				return nil, fmt.Errorf("query is required")
			}
			if err := security.ValidateServiceName(p.Service); err != nil {
				return nil, err
			}

			svc, ok := s.config.Services[p.Service]
			if !ok {
				return nil, fmt.Errorf("unknown service: %s", p.Service)
			}
			if svc.DB == nil {
				return nil, fmt.Errorf("service %q has no db config", p.Service)
			}

			db := svc.DB
			escaped := escapeSingleQuotes(p.Query)
			var cmd string
			switch db.Engine {
			case "postgresql", "postgres":
				cmd = fmt.Sprintf("docker exec %s psql -U %s -d %s -c '%s'",
					svc.Container, db.User, db.Database, escaped)
			case "mysql":
				cmd = fmt.Sprintf("docker exec %s mysql -u %s -p%s %s -e '%s'",
					svc.Container, db.User, db.Password, db.Database, escaped)
			default:
				return nil, fmt.Errorf("unsupported db engine %q", db.Engine)
			}

			res, err := s.transport.Exec(ctx, svc.Host, cmd)
			if err != nil {
				return nil, err
			}
			combined := res.Stdout
			if res.Stderr != "" {
				combined += res.Stderr
			}
			return mcpContent(combined), nil
		},
	})

	s.defs = make([]ToolDef, 0, len(registrations))
	for _, r := range registrations {
		s.defs = append(s.defs, r.def)
		s.tools[r.def.Name] = r.handler
	}
}

// escapeSingleQuotes escapes single quotes for use inside single-quoted shell arguments.
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// resolveHosts returns the host names to target based on optional host/tag filters.
func resolveHosts(s *Server, host, tag string) []string {
	if s.config == nil {
		return nil
	}
	if host != "" {
		if _, ok := s.config.Hosts[host]; ok {
			return []string{host}
		}
		return nil
	}
	if tag != "" {
		return s.config.HostsByTag(tag)
	}
	return s.config.AllHostNames()
}

// serviceLifecycle handles restart/stop/start for a named service.
func serviceLifecycle(ctx context.Context, s *Server, raw json.RawMessage, action string) (any, error) {
	if err := security.CheckPermission(s.profileLevel, config.LevelOperator); err != nil {
		return nil, err
	}

	var p struct {
		Service string `json:"service"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Service == "" {
		return nil, fmt.Errorf("service is required")
	}
	if err := security.ValidateServiceName(p.Service); err != nil {
		return nil, err
	}
	if !p.Confirm {
		return nil, fmt.Errorf("confirm must be true to %s service", action)
	}

	svc, ok := s.config.Services[p.Service]
	if !ok {
		return nil, fmt.Errorf("unknown service: %s", p.Service)
	}

	if s.approver != nil {
		approvalCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		desc := fmt.Sprintf("%s service %s on %s", action, p.Service, svc.Host)
		approved, err := s.approver.RequestApproval(approvalCtx, desc)
		if err != nil {
			return nil, fmt.Errorf("approval request failed: %w", err)
		}
		if !approved {
			return nil, fmt.Errorf("operation denied by approver")
		}
	}

	if s.rateLimiter != nil {
		if err := s.rateLimiter.Check(svc.Host); err != nil {
			return nil, err
		}
	}

	cmd := fmt.Sprintf("docker %s %s", action, svc.Container)
	res, err := s.transport.Exec(ctx, svc.Host, cmd)
	if err != nil {
		return nil, err
	}

	if s.rateLimiter != nil {
		if err := s.rateLimiter.Record(svc.Host, cmd); err != nil {
			fmt.Fprintf(os.Stderr, "mcp: rate limiter record: %v\n", err)
		}
	}
	if s.auditLogger != nil {
		if err := s.auditLogger.Log(security.AuditEntry{
			Command: action,
			Host:    svc.Host,
			Service: p.Service,
			Profile: s.profileLevel.String(),
			Result:  "success",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "mcp: audit log: %v\n", err)
		}
	}

	text := fmt.Sprintf("%s %s: exit_code=%d\n%s", action, svc.Container, res.ExitCode, res.Stdout)
	return mcpContent(text), nil
}
