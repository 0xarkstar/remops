# Changelog

## v0.3.0 (2026-03-29)

### Three-Interface Architecture

remops is now a personal infrastructure gateway with three entry points sharing one security pipeline:

- **CLI** — terminal users
- **MCP** — Claude Code (16 tools)
- **HTTP API** — Hermes, OpenClaw, any AI agent (17 endpoints)

### New Features

- **HTTP API server** (`remops api`) — REST API with Bearer token auth, same security pipeline as CLI/MCP
- **Docker Compose support** — `remops stack {ps,logs,up,pull,restart,down}` across CLI, MCP, and HTTP
- **Container auto-discovery** — `remops discover` scans hosts and adds containers to config
- **Compose stack discovery** — `remops discover --stacks` finds Docker Compose projects
- **MCP auto-setup** — `remops init --mcp` / `remops mcp setup` configures Claude Code automatically
- **Tiered host_exec** — safe commands (uptime, free, docker ps...) run at operator level, others need Telegram approval
- **SSH config integration** — reads `~/.ssh/config` for IdentityFile fallback
- **MCP tool annotations** — readOnlyHint/destructiveHint per MCP spec
- **Output truncation** — 1MB cap prevents unbounded output from overwhelming AI context
- **Command timeout** — 30-second execution timeout on host_exec
- **goreleaser** — cross-platform binaries + Homebrew tap on tag push

### Security Fixes

- MySQL password no longer exposed in process list (uses MYSQL_PWD env var)
- Empty Telegram bot_token/chat_id rejected at config validation
- MCP db_query enforces write permission for mutating queries
- IsWriteQuery catches ALTER, TRUNCATE, CREATE, GRANT, REVOKE, MERGE, CTE-writes
- DetectShellInjection now applied to host_exec commands (was missing)
- ValidateRemotePath for compose stack directories
- ValidateContainerName before docker commands
- Audit log errors logged to stderr (no longer swallowed)
- golangci-lint pinned to v1.62.2

### Improvements

- Approval timeout reads from config instead of hardcoded 5m
- DBConfig validated (engine, user, database required)
- Plugin registration errors logged, command collisions detected
- MCP signal handling (SIGTERM/SIGINT clean shutdown)
- Doctor checks Docker Compose availability
- Code review fixes (scanner errors, mutex safety, type assertions)

## v0.2.1 (2026-03-28)

Audit remediation — 13 bugs fixed, test coverage raised to 60-81%.

## v0.2.0 (2026-03-21)

Initial feature-complete release with CLI, MCP, security pipeline.

## v0.1.0 (2026-03-21)

First release. Basic CLI with status, service, host commands.
