# vpsm server show

Display detailed information about a single server.

## Synopsis

```
vpsm server show [flags]
```

## Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--id` | | `string` | | Server ID to show. Skips interactive selection when provided. |
| `--output` | `-o` | `string` | `table` | Output format. Accepts `table` or `json`. |

### Inherited Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--provider` | `string` | Config default | Cloud provider to use. Resolved from flag or `config.DefaultProvider`. |

## Behavior

The command operates in three modes depending on the flags and terminal context.

### Mode 1: Direct lookup (`--id` provided)

Fetches a single server by ID from the provider and renders it to stdout.

```
vpsm server show --id 12345
vpsm server show --id 12345 -o json
```

### Mode 2: Non-interactive (no `--id`, non-TTY or `--output` explicitly set)

Falls through to `server list` behavior: fetches all servers and renders them as a table or JSON array.

```
vpsm server show -o json | jq '.[] | .name'
vpsm server show | head
```

### Mode 3: Interactive TUI (no `--id`, TTY detected, `--output` not set)

Launches the full-screen Bubbletea TUI application. Starts at the server list view where a server can be selected to navigate into the detail view.

```
vpsm server show
```

The TUI detail view displays:

- **Overview card** -- ID, name, status, provider, type, image, region, created timestamp.
- **Network card** -- Public IPv4, IPv6, private IPv4 (conditional on availability).
- **Metrics charts** -- CPU usage, disk IOPS (read/write), network bandwidth (in/out). Metrics cover the last hour and are fetched asynchronously after the detail loads.

TUI key bindings in the detail view:

| Key | Action |
|-----|--------|
| `q` / `Esc` | Go back to server list |
| `d` | Navigate to delete view |
| `s` | Start or stop the server (toggles based on current status) |
| `r` | Refresh server detail and metrics |
| `c` | SSH connect (requires server running with a public IP) |
| `j` / `k` / `Up` / `Down` | Scroll viewport |
| `PgUp` / `PgDn` / `Ctrl+U` / `Ctrl+D` | Page scroll |

## Output

### Table format (default)

Vertical key-value layout using tab-aligned columns. Conditional fields are omitted when empty.

```
  ID:        12345
  Name:      web-01
  Status:    running
  Provider:  hetzner
  Type:      cx22
  Image:     ubuntu-24.04
  Region:    fsn1
  IPv4:      203.0.113.10
  IPv6:      2001:db8::1
  Private IP: 10.0.0.2
  Created:   2025-11-20 14:30:00 UTC
```

Fields and their display conditions:

| Field | Condition |
|-------|-----------|
| ID | Always |
| Name | Always |
| Status | Always |
| Provider | Always |
| Type | Always |
| Image | Shown when non-empty |
| Region | Always |
| IPv4 | Shown when non-empty |
| IPv6 | Shown when non-empty |
| Private IP | Shown when non-empty |
| Created | Shown when timestamp is non-zero |

### JSON format

The full `Server` struct encoded with 2-space indentation. Fields with `omitempty` are excluded when empty.

```json
{
  "id": "12345",
  "name": "web-01",
  "status": "running",
  "created_at": "2025-11-20T14:30:00Z",
  "public_ipv4": "203.0.113.10",
  "public_ipv6": "2001:db8::1",
  "private_ipv4": "10.0.0.2",
  "region": "fsn1",
  "server_type": "cx22",
  "image": "ubuntu-24.04",
  "provider": "hetzner",
  "metadata": {
    "labels": {"env": "production"}
  }
}
```

JSON field reference:

| JSON key | Type | Optional | Description |
|----------|------|----------|-------------|
| `id` | `string` | No | Provider-assigned server identifier |
| `name` | `string` | No | Server name |
| `status` | `string` | No | Current status (e.g., `running`, `off`, `initializing`) |
| `created_at` | `string` (RFC 3339) | No | Server creation timestamp |
| `public_ipv4` | `string` | Yes | Public IPv4 address |
| `public_ipv6` | `string` | Yes | Public IPv6 address |
| `private_ipv4` | `string` | Yes | Private network IPv4 address |
| `region` | `string` | No | Datacenter or region identifier |
| `server_type` | `string` | No | Instance type (e.g., `cx22`, `cpx31`) |
| `image` | `string` | Yes | OS image name |
| `provider` | `string` | No | Cloud provider name |
| `metadata` | `object` | Yes | Provider-specific fields (labels, volumes, firewalls, etc.) |

## Errors

| Condition | Message | Exit code |
|-----------|---------|-----------|
| Unknown provider | `providers: unknown provider "<name>"` | 1 |
| Invalid server ID (non-numeric for Hetzner) | `failed to fetch server: invalid server ID "<id>": ...` | 1 |
| Server not found | `failed to fetch server: resource not found` | 1 |
| Authentication failure | `failed to fetch server: unauthorized` | 1 |
| Rate limited by provider API | `failed to fetch server: rate limited` | 1 |

In the TUI, errors are displayed inline rather than causing the program to exit. Metrics loading failures are non-blocking; the detail view renders without charts.

## Provider Resolution

The `--provider` flag is resolved in the parent `server` command's `PersistentPreRunE` hook before `show` executes. If the flag is not set, the value falls back to `config.DefaultProvider` from the vpsm configuration file.

## See Also

- `vpsm server list` -- List all servers.
- `vpsm server create` -- Create a new server.
- `vpsm server delete` -- Delete a server.
- `vpsm server metrics` -- View server metrics.
- `vpsm server ssh` -- Connect to a server via SSH.
