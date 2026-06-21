# remops deployment (config-as-code)

Two remops instances with separated roles. One source, git-sha-stamped binaries,
secrets from the environment (Bitwarden Secrets Manager).

| Instance | Where | Invoked by | Profile | Approval |
|----------|-------|-----------|---------|----------|
| **fleet@OCI** | OCI (`remops-bin`) | Hermes (stdio MCP) → SSH → Macs | `operator` | **MultiApprover** (Telegram + Discord, first responder wins) |
| **personal@dev** | dev Mac (`~/go/bin/remops`) | local Claude Code (stdio MCP) | `operator` | **none** (no-approver) — unsafe ops denied, deliberate escalation |

The two approval bots are distinct from the Claude Code Telegram plugin bot. Do
not reuse tokens across them.

## Build & version stamping

Local builds carry a git-derived version (no more bare `dev`):

```bash
make version        # show what will be stamped
make build          # ./remops, stamped
make install-local  # personal@dev binary -> $GOPATH/bin (operator + no-approver)
```

Release builds use goreleaser (`.goreleaser.yaml`) with the same `-X` ldflags.

## Configs

- `fleet-oci.remops.yaml` — fleet@OCI (method: multi)
- `personal-dev.remops.yaml` — personal@dev (no approval section)

Both are templates: placeholder host addresses and `${ENV}` secret references.
Fill placeholders and provide secrets at deploy time. Never commit real
Tailscale IPs, bot tokens, or chat/channel ids.

### Secrets (BWS)

| Env var | Used by | Notes |
|---------|---------|-------|
| `REMOPS_APPROVER_TELEGRAM_TOKEN` | fleet | approver bot (not the CC plugin bot) |
| `REMOPS_APPROVER_TELEGRAM_CHAT_ID` | fleet | supergroup id, `-100…` |
| `REMOPS_APPROVER_DISCORD_TOKEN` | fleet | approver bot |
| `REMOPS_APPROVER_DISCORD_CHANNEL_ID` | fleet | approval channel |
| `REMOPS_APPROVER_DISCORD_OPERATOR_UID` | fleet | operator's Discord user id (allowlist; required) |

`config.Load` expands `${VAR}` from the environment, so inject these from BWS
before launching (`BWS_ACCESS_TOKEN` is already used by Hermes).

## Deploy fleet@OCI

```bash
make deploy-oci OCI_HOST=oci OCI_BIN=/path/to/remops-bin
```

This cross-compiles `linux/arm64` (Graviton), copies the binary, and stops.

### Reload (no gateway restart)

The OCI remops runs as a Hermes (NousResearch hermes-agent) MCP subprocess, so a
new binary only takes effect when the MCP is reloaded. **Do not restart the
Hermes gateway** — co-located agents/profiles (e.g. bluenode) would restart too.

Hermes already supports a hot reload that re-spawns only the MCP subprocesses,
leaving the gateway running: in any Hermes channel send

```
/reload-mcp
```

(`gateway/run.py` `_handle_reload_mcp_command` → `shutdown_mcp_servers()` +
`discover_mcp_tools()`). There is a brief blip while all MCP servers re-spawn,
but the gateway, in-flight agents, and bluenode keep running. Confirm the live
version afterward:

```bash
ssh oci '<remops-bin path> --version'   # expect the git sha you built
```

A full, drain-aware gateway restart (heavier — restarts bluenode too) is
available via `SIGUSR1` to the gateway pid if ever needed; prefer `/reload-mcp`.

## Discord approver setup (one-time)

Add the bot to the server with the `bot` scope; in the approval channel it needs
**View Channel** and **Send Messages**. It posts an approval message with ✅/❌
**buttons** and receives clicks over an **outbound gateway** (websocket)
connection — no Add Reactions, Read Message History, public webhook, or inbound
endpoint required (works behind NAT/Tailscale on OCI).

`discord.allowed_user_ids` is **required** — config validation rejects an empty
list, because Discord channel membership is mutable and not a sufficient
authorization boundary. Only the listed user ids may decide.
