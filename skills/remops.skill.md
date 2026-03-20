---
name: remops
version: 0.1.0
description: Agent-first CLI for remote Docker operations over SSH. Manages containers across multiple hosts with structured output, permission levels, and out-of-band approval for safe AI agent integration.
install: go install github.com/0xarkstar/remops@latest
---

## Commands

### Global Flags

All commands accept these persistent flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--format`, `-f` | `auto` | Output format: `json`, `table`, `auto` |
| `--profile`, `-p` | `admin` | Permission profile to use |
| `--host` | _(all hosts)_ | Target a specific host by name |
| `--tag` | _(all)_ | Filter hosts/services by tag |
| `--verbose`, `-v` | `false` | Verbose output |
| `--sanitize` | `false` | Strip LLM directive injection from output |
| `--timeout` | _(per-host config)_ | Override per-host SSH timeout |
| `--dry-run` | `false` | Show what would happen without executing |

---

### `remops status`

Show container status across all configured hosts (or a subset).

```
remops status [--host <name>] [--tag <tag>] [--format json|table|auto]
```

Queries each host in parallel. Returns system info + container list per host.

**Examples:**
```bash
remops status
remops status --host web
remops status --tag production --format json
```

---

### `remops service`

Manage named services defined in `remops.yaml`.

#### `remops service logs <name>`

Fetch logs for a service. Requires `viewer` permission.

```
remops service logs <name> [--tail N] [--since DURATION] [--follow]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--tail` | `100` | Number of log lines to show |
| `--since` | _(all)_ | Duration or timestamp (e.g. `1h`, `2024-01-01T00:00:00`) |
| `--follow`, `-f` | `false` | Stream log output (like `docker logs -f`) |

**Examples:**
```bash
remops service logs nginx
remops service logs nginx --tail 50 --since 1h
remops service logs app --follow
```

#### `remops service restart <name>`

Restart a service. Requires `operator` permission.

```
remops service restart <name> [--confirm]
```

Without `--confirm`, shows a dry-run preview. With `--confirm`, executes.

**Examples:**
```bash
remops service restart nginx           # dry-run preview
remops service restart nginx --confirm # execute
```

#### `remops service stop <name>`

Stop a service. Requires `operator` permission.

```
remops service stop <name> [--confirm]
```

#### `remops service start <name>`

Start a service. Requires `operator` permission.

```
remops service start <name> [--confirm]
```

---

### `remops host`

Host management commands.

#### `remops host info <name>`

Show system info and containers for a specific host.

```
remops host info <name>
```

**Example:**
```bash
remops host info web
remops host info db --format json
```

---

### `remops doctor`

Run health checks on configuration and connectivity.

```
remops doctor
```

Checks:
- Config file found and valid
- Each host reachable (SSH ping with latency)
- Docker installed on each host
- SSH key file permissions (warns if world/group-readable)
- Telegram bot reachable (if approval configured)
- Audit log directory writable
- Config file permissions

---

### `remops mcp`

Start an MCP (Model Context Protocol) stdio server for Claude Code integration.

```
remops mcp
```

Reads JSON-RPC 2.0 from stdin, writes responses to stdout. All logging goes to stderr. Do not run interactively — configure via `claude.json` (see MCP Server section below).

---

### `remops completion`

Generate shell completion scripts.

```
remops completion [bash|zsh|fish|powershell]
```

**Examples:**
```bash
remops completion zsh > ~/.zsh/completions/_remops
remops completion bash > /etc/bash_completion.d/remops
```

---

## Safety Rules

### Permission Levels

remops enforces three permission levels via profiles:

| Level | Operations Allowed |
|-------|--------------------|
| `viewer` | Read-only: `status`, `service logs`, `host info`, `doctor` |
| `operator` | All viewer ops + write operations (`restart`, `stop`, `start`) with dry-run required and optional out-of-band approval |
| `admin` | Full access, no approval required |

Select a profile with `--profile <name>` (default: `admin`).

### Dry-Run Requirement

Write operations (`restart`, `stop`, `start`) require explicit `--confirm` to execute. Without it, a dry-run preview is shown:

```
[dry-run] would execute on web: docker restart nginx
Pass --confirm to execute this operation.
```

Profiles with `require_dry_run: true` enforce this behavior — the agent cannot bypass it.

### Output Sanitization

Use `--sanitize` to strip LLM prompt injection attempts from Docker container output before it reaches the agent's context. Always use this flag in automated agent pipelines.

### Rate Limiting

The `approval.rate_limit` config block limits write operations per host per hour (default: 5). When exceeded, remops exits with code `7` (rate limited).

### Out-of-Band Approval

When a profile has `approval: out-of-band`, write operations send a Telegram notification requiring human approval before execution. Approval timeout defaults to 5 minutes.

### Audit Logging

All operations are logged to `~/.local/share/remops/audit.log` (XDG data dir). The log is append-only and contains timestamps, user, host, command, and outcome.

---

## Exit Codes

| Code | Constant | Meaning |
|------|----------|---------|
| `0` | `ExitSuccess` | Operation completed successfully |
| `1` | `ExitGeneralError` | General / unhandled error |
| `2` | `ExitPartialFailure` | Some hosts succeeded, some failed |
| `3` | `ExitConfigError` | Configuration file not found or invalid |
| `4` | `ExitConnectionError` | SSH connection failed |
| `5` | `ExitPermissionDenied` | Profile lacks required permission level |
| `6` | `ExitApprovalPending` | Out-of-band approval required but not yet granted |
| `7` | `ExitRateLimited` | Write rate limit exceeded for this host |

---

## Examples

### Check all hosts
```bash
remops status --format json
```

### Stream logs from a service
```bash
remops service logs app --follow
```

### Restart with confirmation (operator profile)
```bash
remops --profile operator service restart nginx --confirm
```

### Dry-run a stop operation
```bash
remops service stop postgres
# [dry-run] would execute on db: docker stop postgres
# Pass --confirm to execute this operation.
```

### Start MCP server (in claude.json)
```bash
remops mcp
```

### Run health checks
```bash
remops doctor
```

### Filter by tag
```bash
remops status --tag production
remops service logs --tag monitoring
```

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `REMOPS_CONFIG` | Path to config file. Overrides default search paths. |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token for out-of-band approval notifications. Referenced in config as `${TELEGRAM_BOT_TOKEN}`. |
| `TELEGRAM_CHAT_ID` | Telegram chat ID to send approval requests to. Referenced in config as `${TELEGRAM_CHAT_ID}`. |

**Config search order:**
1. `$REMOPS_CONFIG`
2. `./remops.yaml`
3. `~/.config/remops/remops.yaml`

---

## MCP Server

remops exposes itself as an MCP server for Claude Code integration. Add to your `claude.json`:

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

Once configured, Claude can call remops tools directly within conversations to inspect and manage remote Docker services.
