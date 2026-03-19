# remops — Project Instructions

## Overview
Agent-first CLI for remote Docker operations over SSH. Go 1.23+, single binary.

## Build & Test
```bash
cd /Users/arkstar/Projects/remops
go build -o remops .
go test ./...
go vet ./...
```

## Architecture
- `cmd/` — Cobra command definitions
- `internal/config/` — YAML config parsing, validation, XDG paths
- `internal/transport/` — SSH abstraction with connection pooling
- `internal/docker/` — Docker CLI wrapping (ps, logs, lifecycle)
- `internal/output/` — JSON/table formatters, response envelope
- `internal/security/` — Permissions, approval, rate limiting, sanitization, audit
- `internal/mcp/` — MCP stdio server for Claude Code integration

## Key Patterns
- Transport interface abstracts SSH vs local execution
- All output through Response envelope (results + failures + summary)
- Permission levels: viewer (read-only), operator (write + approval), admin (full)
- Config loaded from: $REMOPS_CONFIG > ./remops.yaml > ~/.config/remops/remops.yaml
- No Docker client library — wraps `docker` CLI via SSH

## Testing
- Unit tests alongside each package
- Integration tests use mock SSH server (gliderlabs/ssh)
- Run: `go test -race ./...`

## Conventions
- Immutable data patterns where practical
- Error wrapping with context: `fmt.Errorf("context: %w", err)`
- No global mutable state beyond cobra flags
- Exit codes: 0=success, 1=error, 2=partial, 3=config, 4=connection, 5=denied, 6=approval, 7=ratelimit
