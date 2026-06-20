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

`config.Load` expands `${VAR}` from the environment, so inject these from BWS
before launching (`BWS_ACCESS_TOKEN` is already used by Hermes).

## Deploy fleet@OCI

```bash
make deploy-oci OCI_HOST=oci OCI_BIN=/path/to/remops-bin
```

This cross-compiles `linux/arm64` (Graviton), copies the binary, and stops.

### Restart (deliberate — open operational gap)

The OCI remops runs as a Hermes subprocess, so a new binary only takes effect
when the MCP is reloaded. **Do not blind-restart Hermes** — co-located services
(e.g. bluenode) can be disrupted. Prefer reloading only the remops MCP. Confirm
the live version after restart:

```bash
ssh oci '/path/to/remops-bin --version'   # expect the git sha you built
```

## Discord approver setup (one-time)

The Discord bot needs, in the approval channel: **View Channel**, **Send
Messages**, **Add Reactions**, and **Read Message History**. It posts an
approval message, seeds ✅/❌ reactions, and polls reactions — no public
webhook or gateway connection is required.
