# Architecture

**Analysis Date:** 2026-03-20

## Pattern Overview

**Overall:** Domain-first provider architecture with CLI + interactive TUI presentation layer

**Key Characteristics:**
- Multi-domain vertical slices (server, dns, sshkey) each with their own domain/providers/services/tui packages
- Provider pattern with factory-based registry for pluggable cloud backends
- Cobra CLI commands as the entry point, delegating to either non-interactive output or Bubbletea TUI programs
- Shared cross-cutting packages for auth, config, caching, retry, and action tracking

## Layers

**Presentation Layer (cmd/):**
- Purpose: Parse CLI flags, resolve provider, dispatch to service or TUI
- Location: `cmd/root.go`, `cmd/commands/{server,dns,sshkey,auth,config}/`
- Contains: Cobra command definitions, flag handling, output formatting
- Depends on: internal providers, services, tui, config, auth
- Used by: `main.go` entry point

**TUI Layer (internal/*/tui/):**
- Purpose: Interactive terminal UI for guided workflows
- Location: `internal/server/tui/`, `internal/dns/tui/`, `internal/sshkey/tui/`
- Contains: Bubbletea models with message-based navigation between views
- Depends on: domain types, services, shared TUI components (`internal/tui/`)
- Used by: cmd layer when running in interactive mode

**Service Layer (internal/*/services/):**
- Purpose: Business logic, input validation, normalisation, caching
- Location: `internal/dns/services/`, `internal/server/services/action/`
- Contains: Service structs wrapping providers with validation and caching
- Depends on: domain interfaces, swrcache
- Used by: cmd layer, TUI layer

**Domain Layer (internal/*/domain/):**
- Purpose: Define provider interfaces and domain types
- Location: `internal/server/domain/`, `internal/dns/domain/`, `internal/sshkey/domain/`
- Contains: Provider interfaces, entity structs, error sentinel values
- Depends on: `internal/domain/` for shared errors, `internal/platform/sshkey/` for shared SSH key spec
- Used by: All layers

**Provider Layer (internal/*/providers/):**
- Purpose: Implement provider interfaces against real cloud APIs
- Location: `internal/server/providers/`, `internal/dns/providers/`, `internal/sshkey/providers/`
- Contains: Factory registries and concrete provider implementations (Hetzner, Porkbun, Cloudflare)
- Depends on: domain interfaces, auth store, external SDKs
- Used by: cmd layer via registry lookup

**Infrastructure Layer (internal/):**
- Purpose: Cross-cutting concerns shared across domains
- Location: `internal/services/auth/`, `internal/config/`, `internal/actionstore/`, `internal/swrcache/`, `internal/retry/`, `internal/cache/`
- Contains: Auth store (OS keyring), JSON config, SQLite action persistence, SWR cache, retry utilities
- Depends on: OS APIs, go-keyring, modernc/sqlite
- Used by: All layers

## Data Flow

**CLI Command Execution (non-interactive):**

1. `main.go` calls `cmd.Execute()` which registers all providers into global registries
2. Cobra routes to a command handler (e.g. `server/list.go:runList`)
3. `PersistentPreRunE` resolves the `--provider` flag from CLI or config default
4. Handler calls `providers.Get(name, auth.DefaultStore())` to construct a provider via factory
5. Handler calls provider methods directly (e.g. `provider.ListServers(ctx)`)
6. Results are formatted as table or JSON and printed to stdout

**Interactive TUI Flow (server):**

1. Same steps 1-4 as above
2. Handler detects TTY and launches `tui.RunServerApp(provider, providerName)`
3. Bubbletea program manages view transitions via messages (list -> show -> delete -> create)
4. TUI calls provider methods and renders results interactively

**DNS Command with Service Layer:**

1. `dns.go:newDNSService(cmd)` resolves provider and wraps it in `services.New(provider)`
2. Service normalises inputs (domain names, subdomains), validates record types/content
3. Service delegates to underlying provider, optionally using SWR cache for reads
4. Cache is invalidated on writes (create/update/delete)

**Action Tracking (server start/stop):**

1. Server command triggers an action (e.g. `provider.StartServer(ctx, id)`)
2. `action.Service.TrackAction()` persists an `ActionRecord` to SQLite
3. `action.Service.WaitForAction()` polls either via `ActionPoller` interface (action-based) or `GetServer` (status-based)
4. On completion, `FinalizeAction()` updates the record to success/error
5. Old completed records are cleaned up opportunistically (24h TTL)

**State Management:**
- Auth tokens: OS keyring via `go-keyring` (per-provider, keyed by normalised provider name)
- User config: JSON file at `~/.config/vpsm/config.json` (default provider, DNS provider)
- Action records: SQLite at `~/.config/vpsm/vpsm.db` (in-flight action tracking)
- Server prefs: SQLite at `~/.config/vpsm/vpsm.db` (SSH usernames per server)
- DNS cache: File-backed JSON in OS user cache dir (`~/Library/Caches/vpsm/dns/` on macOS)

## Key Abstractions

**Provider Interface (per domain):**
- Purpose: Pluggable cloud provider backends
- Examples: `internal/server/domain/provider.go`, `internal/dns/domain/provider.go`, `internal/sshkey/domain/provider.go`
- Pattern: Interface + factory registry. Each domain has its own `Provider` interface and `providers.Registry` with `Register(name, factory)` / `Get(name, store)`. Providers are registered at startup in `cmd/root.go:Execute()`.

**Extended Provider Interfaces (server domain):**
- Purpose: Optional capabilities beyond basic CRUD
- Examples: `CatalogProvider` (list locations/types/images), `ActionPoller` (poll action progress), `MetricsProvider` (server metrics), `SSHKeyManager` (SSH key CRUD)
- Pattern: Composable interfaces extending the base `Provider`. Consumers type-assert at runtime: `if poller, ok := provider.(domain.ActionPoller); ok { ... }`

**Factory Registry:**
- Purpose: Decouple provider construction from usage
- Examples: `internal/server/providers/registry.go`, `internal/dns/providers/registry.go`, `internal/sshkey/providers/registry.go`
- Pattern: `map[string]Factory` with mutex-protected Register/Get. Factory type is `func(store auth.Store) (domain.Provider, error)`. Registration happens at startup, panics on duplicates.

**Auth Store:**
- Purpose: Abstract credential storage
- Examples: `internal/services/auth/auth.go`, `internal/services/auth/keyring_auth.go`
- Pattern: `Store` interface with `SetToken/GetToken/DeleteToken`. Default impl uses OS keyring. Mock impl exists for tests (`internal/services/auth/mock_store.go`).

**SWR Cache:**
- Purpose: Stale-while-revalidate caching for API reads
- Examples: `internal/swrcache/cache.go`
- Pattern: Generic `GetOrFetch[T]` function. Returns cached data if fresh, returns stale data + background refresh if within max stale window, fetches fresh otherwise. File-backed JSON storage with atomic writes.

## Entry Points

**main.go:**
- Location: `main.go`
- Triggers: CLI invocation (`vpsm <command>`)
- Responsibilities: Calls `cmd.Execute()`

**cmd.Execute():**
- Location: `cmd/root.go`
- Triggers: Called by `main.go`
- Responsibilities: Registers all providers (Hetzner server, Hetzner SSH key, Porkbun DNS, Cloudflare DNS), builds Cobra command tree, executes root command

**Provider Registration:**
- Location: `internal/server/providers/hetzner_catalog.go`, `internal/dns/providers/porkbun.go`, `internal/dns/providers/cloudflare.go`, `internal/sshkey/providers/hetzner.go`
- Triggers: Called by `cmd.Execute()` at startup
- Responsibilities: Register factory functions that construct providers from auth store

## Error Handling

**Strategy:** Sentinel errors + wrapping with `fmt.Errorf("%w", err)`

**Patterns:**
- Shared sentinel errors in `internal/domain/errors.go`: `ErrNotFound`, `ErrUnauthorized`, `ErrRateLimited`, `ErrConflict`
- Domain packages re-export shared sentinels (e.g. `internal/server/domain/errors.go` aliases `shared.ErrNotFound`)
- CLI commands print errors to stderr and return (no `os.Exit(1)` in subcommands)
- Service layer returns validation errors as plain `fmt.Errorf()` strings
- Action polling distinguishes rate-limit errors (abort immediately) from transient errors (retry up to 3 times)

## Cross-Cutting Concerns

**Logging:** No structured logging framework. Progress/error messages written to `cmd.ErrOrStderr()` via `fmt.Fprintf`. TUI handles its own display.

**Validation:** Service layer validates inputs (record types, domain names, server names). `internal/util/validate.go` has shared validators. `internal/dns/services/validate.go` has DNS-specific validation.

**Authentication:** OS keyring via `go-keyring`. Provider name normalised via `util.NormalizeKey()` (lowercase, trimmed). Token retrieved at provider construction time via factory. No session tokens or refresh flow.

**Retry:** `internal/retry/retry.go` provides configurable retry with exponential backoff and jitter. Used by provider implementations for transient network errors. Default: 3 attempts, 500ms base delay, 5s max delay.

**Caching:** SWR cache (`internal/swrcache/`) for DNS reads. 5min fresh TTL, 1hr max stale. File-backed JSON. Cache invalidated on mutations.

---

*Architecture analysis: 2026-03-20*
