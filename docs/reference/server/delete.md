# vpsm server delete

Delete a server instance from the specified cloud provider. The server is permanently removed; this operation cannot be undone.

## Synopsis

```
vpsm server delete [flags]
```

## Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--id` | | `string` | | Server ID to delete. Skips interactive selection when provided. Required when stdout is not an interactive terminal. |

### Inherited Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--provider` | `string` | Config default | Cloud provider to use. Resolved from flag or `config.DefaultProvider`. |

## Behavior

The command operates in two modes depending on the flags and terminal context.

### Mode 1: Non-interactive (`--id` provided)

Deletes the specified server directly with no prompts. Use this mode in scripts and CI pipelines.

```
vpsm server delete --provider hetzner --id 12345
```

### Mode 2: Interactive TUI (no `--id`, TTY detected)

Launches a full-screen TUI that presents the current server list. After selecting a server, a confirmation prompt is displayed before any deletion occurs. Cancelling at any point exits cleanly with no deletion.

```
vpsm server delete --provider hetzner
```

You must not omit `--id` when stdout is not an interactive terminal; the command exits with an error.

## Output

| Stream | Content |
|--------|---------|
| stderr | Progress message: `Deleting server "<name>" (ID: <id>)...` (interactive) or `Deleting server <id>...` (non-interactive) |
| stdout | `Server <id> deleted successfully.` on success |
| stderr | `Server deletion cancelled.` when the interactive TUI is dismissed without confirming |
| stderr | Error message on failure |

## Errors

| Condition | Message | Exit code |
|-----------|---------|-----------|
| `--provider` not set and no default configured | `no provider specified: use --provider flag or set a default with 'vpsm config set default-provider <name>'` | 1 |
| Unknown provider | `providers: unknown provider "<name>"` | 1 |
| `--id` omitted outside a terminal | `--id is required when not running in a terminal` | 1 |
| Invalid server ID (non-numeric for Hetzner) | `failed to delete server: invalid server ID "<id>": ...` | 1 |
| Server not found | `failed to delete server: resource not found` | 1 |
| Authentication failure | `failed to delete server: unauthorized` | 1 |
| Rate limited by provider API | `failed to delete server: rate limited` | 1 |

Interactive TUI cancellation exits with status 0.

## Provider Resolution

The `--provider` flag is resolved in the parent `server` command's `PersistentPreRunE` hook before `delete` executes. If the flag is not set, the value falls back to `config.DefaultProvider` from the vpsm configuration file.

## Provider Notes

**Hetzner** — server IDs are numeric strings. Supplying a non-numeric value to `--id` returns an `invalid server ID` error before any network call is made. Transient API errors are retried automatically with a 30-second per-attempt timeout.

## See Also

- `vpsm server list` -- List all servers and their IDs.
- `vpsm server show` -- Inspect a specific server before deletion.
- `vpsm server create` -- Create a new server.
- `vpsm config set default-provider` -- Configure a default provider.
