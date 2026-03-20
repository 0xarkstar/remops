# remops

[![CI](https://github.com/0xarkstar/remops/actions/workflows/ci.yml/badge.svg)](https://github.com/0xarkstar/remops/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/0xarkstar/remops)](https://goreportcard.com/report/github.com/0xarkstar/remops)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**One CLI, all your servers. Built for AI agents. Designed for humans.**

remops manages Docker containers across multiple remote hosts via SSH. It provides structured JSON output, layered permission levels, and out-of-band human approval — making it safe to hand to an AI agent without giving it unchecked access to production.

---

## Features

- **Multi-host operations** — run commands across all hosts (or filter by tag) in parallel
- **Structured output** — JSON envelopes with results, failures, and summary; machine-readable by default
- **Permission profiles** — viewer (read-only), operator (write + approval), admin (full access)
- **Dry-run by default** — write operations show preview unless `--confirm` is passed
- **Out-of-band approval** — Telegram notifications with human approval before destructive actions
- **MCP server** — expose remops as a Claude Code tool via Model Context Protocol
- **Safety-first** — rate limiting, output sanitization, audit logging, SSH key checks

---

## Quick Start

### Install

```bash
go install github.com/0xarkstar/remops@latest
```

Or download a binary from the [releases page](https://github.com/0xarkstar/remops/releases).

### Create a config

```bash
mkdir -p ~/.config/remops
cat > ~/.config/remops/remops.yaml << 'EOF'
version: 1

hosts:
  web:
    address: 1.2.3.4
    user: deploy
    tags: [production]

services:
  nginx:
    host: web
    container: nginx

profiles:
  viewer:
    level: viewer
  operator:
    level: operator
    require_dry_run: true
  admin:
    level: admin
EOF
```

### First run

```bash
# Check connectivity and config
remops doctor

# View container status across all hosts
remops status

# View service logs
remops service logs nginx

# Dry-run a restart (safe — shows preview only)
remops service restart nginx

# Actually restart (requires --confirm)
remops service restart nginx --confirm
```

---

## Configuration

remops looks for config in this order:
1. `$REMOPS_CONFIG` environment variable (path to file)
2. `./remops.yaml` (current directory)
3. `~/.config/remops/remops.yaml` (XDG config dir)

### Full `remops.yaml` reference

```yaml
version: 1

hosts:
  web:
    address: 1.2.3.4        # IP or hostname (required)
    user: deploy             # SSH user (default: root)
    port: 22                 # SSH port (default: 22)
    key: ~/.ssh/id_ed25519   # Private key path (default: SSH agent)
    ssh_config: ~/.ssh/config # Use an SSH config file
    proxy_jump: bastion      # ProxyJump host (name or address)
    timeout: 10s             # Per-host SSH timeout (default: 10s)
    description: "Web server"
    tags: [production, web]
    meta:                    # Arbitrary key-value metadata
      region: us-east-1

services:
  nginx:
    host: web                # Host name (required)
    container: nginx         # Docker container name (required)
    tags: [proxy]

profiles:
  viewer:
    level: viewer            # viewer | operator | admin
  operator:
    level: operator
    require_dry_run: true    # Force dry-run preview before execution
    approval: out-of-band    # Require Telegram approval
  admin:
    level: admin

approval:
  method: telegram
  bot_token: ${TELEGRAM_BOT_TOKEN}   # Env var interpolation supported
  chat_id: ${TELEGRAM_CHAT_ID}
  timeout: 5m                         # Approval window (default: 5m)
  rate_limit:
    max_writes_per_host_per_hour: 5   # Default: 5
```

---

## Commands

### `remops status`

Show container status across all configured hosts in parallel.

```bash
remops status
remops status --host web
remops status --tag production --format json
```

### `remops service`

Manage named services defined in config.

```bash
# Logs (viewer permission)
remops service logs nginx
remops service logs nginx --tail 200 --since 1h
remops service logs app --follow

# Write operations (operator permission, --confirm required)
remops service restart nginx           # dry-run preview
remops service restart nginx --confirm # execute
remops service stop nginx --confirm
remops service start nginx --confirm
```

### `remops host`

Host-specific operations.

```bash
remops host info web
remops host info db --format json
```

### `remops doctor`

Run health checks on config, SSH connectivity, Docker availability, and key permissions.

```bash
remops doctor
```

Output:
```
Check                                      Status Detail
--------------------------------------------------------------------------------
Config file                                PASS   found and valid
Host web reachable                         PASS   latency 3ms
Docker on web                              PASS   Docker version 26.1.0
SSH key permissions                        PASS   key files have safe permissions
Audit log dir                              PASS   /Users/you/.local/share/remops
Config file permissions                    PASS   ~/.config/remops/remops.yaml (0600)
```

### `remops mcp`

Start the MCP stdio server for Claude Code integration. See [MCP Integration](#mcp-integration).

### `remops completion`

Generate shell completions.

```bash
remops completion zsh > ~/.zsh/completions/_remops
remops completion bash > /etc/bash_completion.d/remops
```

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--format`, `-f` | `auto` | Output format: `json`, `table`, `auto` |
| `--profile`, `-p` | `admin` | Permission profile |
| `--host` | all | Target specific host |
| `--tag` | all | Filter by tag |
| `--verbose`, `-v` | `false` | Verbose output |
| `--sanitize` | `false` | Strip LLM injection from container output |
| `--timeout` | config | Override per-host SSH timeout |
| `--dry-run` | `false` | Preview without executing |

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Partial failure (some hosts failed) |
| `3` | Config error |
| `4` | Connection error |
| `5` | Permission denied |
| `6` | Approval pending |
| `7` | Rate limited |

---

## MCP Integration

remops ships with a built-in MCP (Model Context Protocol) server. Configure it in your Claude Code `claude.json`:

```json
{
  "mcpServers": {
    "remops": {
      "command": "remops",
      "args": ["mcp"]
    }
  }
}
```

Once configured, Claude can call remops tools directly within conversations — checking container status, reading logs, or requesting restarts (with appropriate approval flow).

---

## Security Model

### Permission Profiles

Three levels of access, configured in `remops.yaml` and selected with `--profile`:

| Profile | Level | What it can do |
|---------|-------|----------------|
| `viewer` | Read-only | `status`, `service logs`, `host info`, `doctor` |
| `operator` | Write + approval | All viewer ops + `restart`, `stop`, `start` (with confirmation) |
| `admin` | Full | All operations, no approval required |

### Dry-Run by Default

Write operations always show a preview first:

```
[dry-run] would execute on web: docker restart nginx
Pass --confirm to execute this operation.
```

Profiles with `require_dry_run: true` enforce this even when `--dry-run` is not passed explicitly.

### Out-of-Band Approval

When a profile includes `approval: out-of-band`, write operations send a Telegram message to the configured chat before executing. The operation waits for human approval within the configured timeout (default: 5 minutes). If no approval arrives, remops exits with code `6`.

### Rate Limiting

Write operations are rate-limited per host. The default limit is 5 writes per host per hour. When exceeded, remops exits with code `7`. Configure with `approval.rate_limit.max_writes_per_host_per_hour`.

### Output Sanitization

Use `--sanitize` to strip LLM prompt injection attempts from Docker output before it reaches the agent context. Recommended for all automated pipelines.

### Audit Logging

All operations are appended to `~/.local/share/remops/audit.log`, including timestamp, profile, host, command, and outcome.

---

## Architecture

```
CLI (Cobra)
    │
    ├── status / service / host / doctor / mcp
    │
    ▼
Transport Layer (internal/transport)
    │   SSH connection pool, Exec, Stream, Ping
    │
    ▼
SSH (golang.org/x/crypto/ssh)
    │
    ▼
Docker CLI (on remote host)
    │   docker ps, docker logs, docker restart/stop/start
    │
    ▼
Output (internal/output)
        Response envelope: results + failures + summary
        Formatters: JSON, table, auto
```

**Key packages:**

| Package | Role |
|---------|------|
| `cmd/` | Cobra command definitions and flag parsing |
| `internal/config/` | YAML config loading, validation, XDG paths |
| `internal/transport/` | SSH abstraction with connection pooling |
| `internal/docker/` | Docker CLI wrapping (ps, logs, lifecycle) |
| `internal/output/` | JSON/table formatters, response envelope |
| `internal/security/` | Permission checks, rate limiting, sanitization, audit |
| `internal/mcp/` | MCP stdio server (JSON-RPC 2.0) |

---

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Run tests: `go test -race ./...`
4. Run vet: `go vet ./...`
5. Submit a pull request

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
