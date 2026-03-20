package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
			{"type": "text", "text": text},
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
				var p struct {
					Host string `json:"host"`
					Tag  string `json:"tag"`
				}
				_ = json.Unmarshal(raw, &p)

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
				var p struct {
					Service string `json:"service"`
					Tail    int    `json:"tail"`
					Since   string `json:"since"`
				}
				if err := json.Unmarshal(raw, &p); err != nil || p.Service == "" {
					return nil, fmt.Errorf("service is required")
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
				var p struct {
					Host string `json:"host"`
				}
				if err := json.Unmarshal(raw, &p); err != nil || p.Host == "" {
					return nil, fmt.Errorf("host is required")
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
				Name:        "remops_doctor",
				Description: "Run health checks: ping all hosts and verify Docker is accessible.",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
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

	s.defs = make([]ToolDef, 0, len(registrations))
	for _, r := range registrations {
		s.defs = append(s.defs, r.def)
		s.tools[r.def.Name] = r.handler
	}
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
	var p struct {
		Service string `json:"service"`
		Confirm bool   `json:"confirm"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Service == "" {
		return nil, fmt.Errorf("service is required")
	}
	if !p.Confirm {
		return nil, fmt.Errorf("confirm must be true to %s service", action)
	}

	svc, ok := s.config.Services[p.Service]
	if !ok {
		return nil, fmt.Errorf("unknown service: %s", p.Service)
	}

	cmd := fmt.Sprintf("docker %s %s", action, svc.Container)
	res, err := s.transport.Exec(ctx, svc.Host, cmd)
	if err != nil {
		return nil, err
	}
	text := fmt.Sprintf("%s %s: exit_code=%d\n%s", action, svc.Container, res.ExitCode, res.Stdout)
	return mcpContent(text), nil
}
