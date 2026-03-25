# remops

[![CI](https://github.com/0xarkstar/remops/actions/workflows/ci.yml/badge.svg)](https://github.com/0xarkstar/remops/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/0xarkstar/remops)](https://goreportcard.com/report/github.com/0xarkstar/remops)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Manage your servers from terminal, Claude Code, or Telegram — through one security pipeline.**

remops is a single binary that gives you three ways to manage Docker containers across your servers:

```
Terminal ──→ CLI ────┐
Claude  ──→ MCP ────┤→ [auth → validate → approve → execute → audit] → your servers
Hermes  ──→ HTTP ───┘
```

You type in terminal. Claude calls MCP tools. Your Telegram bot hits the HTTP API. All three go through the same permission checks, Telegram approval, rate limiting, and audit logging.

---

## Why remops?

**If you run 2-5 personal servers and use AI agents**, you've probably done this:

```bash
# Which server was crawl4ai on again?
ssh prod "docker ps | grep crawl4ai"
# Or maybe it was dev?
ssh dev "docker ps | grep crawl4ai"
```

With remops:

```bash
remops logs crawl4ai --tail 20
```

You don't need to remember which server. You don't need to remember docker commands. And when Claude or your Telegram bot asks about your servers, they use the same tool — safely.

**What makes remops different from SSH + aliases:**

| | SSH | remops |
|---|---|---|
| Multi-host status | Loop over each server | `remops status` — one command, all hosts |
| Service by name | Remember server + container name | `remops logs nginx` — just the service name |
| AI agent access | Give it raw SSH (dangerous) | Scoped tools with approval |
| Audit trail | Shell history | Structured JSONL log |
| Access from Telegram | Not possible | HTTP API with same security |

---

## Quick Start

### 1. Install

```bash
go install github.com/0xarkstar/remops@latest
```

### 2. Initialize

```bash
remops init
```

This creates `~/.config/remops/remops.yaml` interactively. If you have `~/.ssh/config`, remops reads it for host defaults.

### 3. Verify

```bash
remops doctor
```

```
Check                                      Status Detail
--------------------------------------------------------------------------------
Config file                                PASS   found and valid
Host prod reachable                        PASS   latency 13ms
Docker on prod                             PASS   Docker version 28.5.2
SSH key permissions                        PASS   key files have safe permissions
```

### 4. Use

```bash
# See all containers across all servers
remops status

# Read service logs
remops service logs crawl4ai --tail 50

# Restart a service (requires --confirm)
remops service restart nginx --confirm
```

---

## Three Interfaces

### CLI — for humans in terminal

```bash
remops status
remops service logs nginx --tail 100 --since 1h
remops service restart nginx --confirm
remops host disk prod
```

### MCP — for Claude Code

Add to your `~/.claude.json`:

```json
{
  "mcpServers": {
    "remops": {
      "command": "remops",
      "args": ["mcp", "--profile", "operator"]
    }
  }
}
```

Then in Claude Code:

```
You: "Check my server status"
Claude: [calls remops_status] → "prod: 5 containers running, dev: no containers"

You: "Restart crawl4ai"
Claude: [calls remops_service_restart] → Telegram approval → restarted
```

**10 MCP tools:** `remops_status`, `remops_service_logs`, `remops_service_restart`, `remops_service_stop`, `remops_service_start`, `remops_host_info`, `remops_host_disk`, `remops_host_exec`, `remops_doctor`, `remops_db_query`

### HTTP API — for any AI agent

```yaml
# In remops.yaml
api:
  listen: ":9090"
  api_key: ${REMOPS_API_KEY}
```

```bash
remops api --profile operator
```

Now any AI agent (Hermes, OpenClaw, custom bots) can manage your servers:

```bash
# Status
curl -H "Authorization: Bearer $KEY" http://localhost:9090/api/v1/status

# Logs
curl -H "Authorization: Bearer $KEY" http://localhost:9090/api/v1/services/nginx/logs?tail=50

# Restart (with approval flow)
curl -X POST -H "Authorization: Bearer $KEY" \
  -d '{"confirm":true}' http://localhost:9090/api/v1/services/nginx/restart

# Doctor
curl -H "Authorization: Bearer $KEY" http://localhost:9090/api/v1/doctor
```

**All endpoints:**

| Method | Endpoint | Permission | Description |
|--------|----------|------------|-------------|
| GET | `/api/v1/status` | viewer | Container status across hosts |
| GET | `/api/v1/services/:name/logs` | viewer | Service logs |
| POST | `/api/v1/services/:name/restart` | operator | Restart service |
| POST | `/api/v1/services/:name/stop` | operator | Stop service |
| POST | `/api/v1/services/:name/start` | operator | Start service |
| GET | `/api/v1/hosts/:name/info` | viewer | Host system info |
| GET | `/api/v1/hosts/:name/disk` | viewer | Disk usage |
| POST | `/api/v1/hosts/:name/exec` | admin | Execute command |
| GET | `/api/v1/doctor` | viewer | Health check |
| POST | `/api/v1/db/:service/query` | viewer/operator | SQL query |
| GET | `/api/v1/version` | viewer | Version info |

---

## Security

All three interfaces share the same security pipeline:

```
Request → Auth → Permission Check → Input Validation → Approval → Rate Limit → Execute → Sanitize → Audit → Response
```

### Permission Profiles

| Profile | Can do | Can't do |
|---------|--------|----------|
| **viewer** | status, logs, host info, doctor | restart, stop, start, exec |
| **operator** | + restart, stop, start (with approval) | exec |
| **admin** | everything | — |

### Telegram Approval

Write operations send a message to your Telegram group with Approve/Deny buttons. The operation blocks until you respond or the timeout (default: 5m) expires.

```yaml
approval:
  method: telegram
  bot_token: ${TELEGRAM_BOT_TOKEN}
  chat_id: ${TELEGRAM_CHAT_ID}
  timeout: 5m
  rate_limit:
    max_writes_per_host_per_hour: 5
```

### Rate Limiting

Write operations are limited per host (default: 5/hour). Prevents runaway agent loops.

### Output Sanitization

Container output is stripped of LLM prompt injection patterns (`SYSTEM:`, `IGNORE PREVIOUS`, etc.) before reaching AI agents.

### Audit Logging

Every operation is logged to `~/.local/share/remops/audit.log` with timestamp, profile, host, command, and result.

---

## Configuration

```yaml
version: 1

hosts:
  prod:
    address: 100.91.194.29     # IP or hostname (required)
    user: deploy                # SSH user (default: root)
    port: 22                    # SSH port (default: 22)
    key: ~/.ssh/id_ed25519     # Key path (also reads ~/.ssh/config)
    proxy_jump: bastion         # ProxyJump host
    timeout: 5s                 # SSH timeout (default: 10s)
    tags: [production]
    meta:
      region: us-east-1

services:
  nginx:
    host: prod                  # Host name (required)
    container: nginx            # Container name (required)
    tags: [web]
    db:                         # Optional DB config for remops db query
      engine: postgresql        # postgresql or mysql
      user: admin
      database: mydb

profiles:
  viewer:
    level: viewer
  operator:
    level: operator
  admin:
    level: admin

approval:                       # Optional
  method: telegram
  bot_token: ${TELEGRAM_BOT_TOKEN}
  chat_id: ${TELEGRAM_CHAT_ID}
  timeout: 5m
  rate_limit:
    max_writes_per_host_per_hour: 5

api:                            # Optional — enables HTTP API
  listen: ":9090"
  api_key: ${REMOPS_API_KEY}
```

Config search order: `$REMOPS_CONFIG` → `./remops.yaml` → `~/.config/remops/remops.yaml`

Environment variables are supported: `${VAR}` or `${VAR:-default}`.

---

## Architecture

```
                    ┌─────────┐
Terminal ──→ CLI ──→│         │
Claude  ──→ MCP ──→│ Security│──→ SSH Transport ──→ Docker CLI
Agent   ──→ HTTP ──→│Pipeline │    (conn pool)      (on remote)
                    └─────────┘
```

**Single binary. Zero dependencies on remote hosts.** Just SSH + Docker on the target.

| Package | Role |
|---------|------|
| `cmd/` | Cobra commands (status, service, host, mcp, api, doctor) |
| `internal/api/` | HTTP REST server |
| `internal/mcp/` | MCP stdio server (JSON-RPC 2.0) |
| `internal/transport/` | SSH abstraction with connection pooling |
| `internal/security/` | Permissions, approval, rate limiting, sanitization, audit |
| `internal/config/` | YAML config, validation, env var interpolation |
| `internal/docker/` | Docker CLI output parsing |
| `internal/output/` | JSON/table formatters |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General error |
| `2` | Partial failure (some hosts failed) |
| `3` | Config error |
| `4` | Connection error |
| `5` | Permission denied |
| `6` | Approval pending/timeout |
| `7` | Rate limited |

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
