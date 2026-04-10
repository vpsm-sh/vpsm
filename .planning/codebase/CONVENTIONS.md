# Coding Conventions

**Analysis Date:** 2026-03-20

## Naming Patterns

**Files:**
- Use `snake_case.go` for all Go source files (e.g., `server_app.go`, `cache_test.go`, `mock_store.go`)
- Test files use `_test.go` suffix co-located with the source they test
- Domain types live in files named after the concept: `server.go`, `provider.go`, `errors.go`

**Functions:**
- Use `PascalCase` for exported functions: `NewCommand()`, `ListServers()`, `RegisterHetzner()`
- Use `camelCase` for unexported functions: `toDomainServer()`, `isHetznerRetryable()`, `backoffDelay()`
- Constructor functions follow `New<Type>()` pattern: `NewHetznerProvider()`, `NewMockStore()`, `NewCommand()`
- Command constructors return `*cobra.Command` and follow `<Verb>Command()`: `ListCommand()`, `DeleteCommand()`, `CreateCommand()`
- Command run functions use `run<Verb>` pattern: `runList()`, `runDelete()`

**Variables:**
- Use `camelCase` for local variables and struct fields
- Package-level sentinel errors use `Err<Name>` pattern: `ErrNotFound`, `ErrUnauthorized`, `ErrRateLimited`
- Constants use `camelCase` for unexported, `PascalCase` for exported: `requestTimeout`, `DefaultTTL`, `ServiceName`

**Types:**
- Interfaces use descriptive nouns: `Provider`, `Store`, `CatalogProvider`, `ActionPoller`
- Structs use `PascalCase` nouns: `HetznerProvider`, `Config`, `Server`, `MockStore`
- Type aliases for functions: `Factory`, `Predicate`

**Packages:**
- Use short, lowercase, single-word names: `domain`, `providers`, `services`, `cache`, `retry`, `util`, `auth`
- Import aliases use descriptive names when collisions occur: `cfgcmd`, `dnsproviders`, `serverproviders`, `shared`

## Code Style

**Formatting:**
- Standard `gofmt` formatting (implicit via Go toolchain)
- No custom formatter configuration detected

**Linting:**
- `go vet` via `make lint`
- `staticcheck` (optional, runs if installed)
- No `.golangci.yml` configuration file — uses tool defaults

## Import Organization

**Order (standard Go convention, enforced by goimports):**
1. Standard library imports (`context`, `fmt`, `errors`, `encoding/json`, `net/http`)
2. Internal project imports (`nathanbeddoewebdev/vpsm/internal/...`)
3. Third-party imports (`github.com/spf13/cobra`, `github.com/hetznercloud/hcloud-go/v2/hcloud`)

**Path Aliases:**
- Use aliases to disambiguate collisions:
  - `cfgcmd "nathanbeddoewebdev/vpsm/cmd/commands/config"` in `cmd/root.go`
  - `dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"` in `cmd/root.go`
  - `shared "nathanbeddoewebdev/vpsm/internal/domain"` in domain error re-exports

**Module Path:** `nathanbeddoewebdev/vpsm`

## Error Handling

**Sentinel Errors:**
- Define shared sentinel errors in `internal/domain/errors.go`: `ErrNotFound`, `ErrUnauthorized`, `ErrRateLimited`, `ErrConflict`
- Feature-specific domain packages re-export from shared domain: see `internal/server/domain/errors.go` and `internal/dns/domain/errors.go`
- Auth has its own sentinel: `ErrTokenNotFound` in `internal/services/auth/auth.go`

**Error Wrapping:**
- Always wrap errors with context using `fmt.Errorf("action: %w", err)` pattern
- Provider methods wrap SDK errors into sentinel errors so CLI layer does not import SDK
- Example pattern from `internal/server/providers/hetzner.go`:
```go
if hcloud.IsError(err, hcloud.ErrorCodeNotFound) {
    return fmt.Errorf("failed to delete server: %w", domain.ErrNotFound)
}
```

**Error Propagation:**
- CLI command handlers write errors to `cmd.ErrOrStderr()` using `fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)`
- Functions return `error` as the last return value (standard Go)
- Validation errors return immediately with descriptive message: `fmt.Errorf("invalid server ID %q: %w", id, err)`

**Panics:**
- Used only for programmer errors in init-time registration: `panic("providers: empty provider name")`
- Never used in runtime code paths

## Logging

**Framework:** No logging framework. Uses `fmt.Fprintf` to stderr for error output in CLI commands.

**Patterns:**
- No structured logging detected
- Errors are surfaced directly to the user via stderr
- TUI components handle errors internally through Bubbletea message passing

## Comments

**When to Comment:**
- Package-level doc comments on every package (e.g., `// Package config handles persistent user configuration for vpsm.`)
- Exported types and functions always have doc comments
- Comments explain "why" not "what" for non-obvious logic
- Section dividers in test files use `// --- Section Name ---` format

**JSDoc/TSDoc:** Not applicable (Go project)

**Go Doc Style:**
```go
// DeleteServer removes a server by its ID. The ID must be a numeric string
// matching the Hetzner server ID.
func (h *HetznerProvider) DeleteServer(ctx context.Context, id string) error {
```

## Function Design

**Size:** Most functions are under 40 lines. TUI update/view methods are longer (up to ~100 lines) due to Bubbletea pattern requirements.

**Parameters:**
- Use `context.Context` as first parameter for any I/O operation
- Use option structs for complex creation: `domain.CreateServerOpts`, `domain.CreateRecordOpts`
- Use variadic functional options for provider construction: `NewHetznerProvider(opts ...hcloud.ClientOption)`

**Return Values:**
- `(result, error)` for operations that can fail
- Single `error` for operations with no meaningful result (e.g., `DeleteServer`)
- Pointer returns for optional/nullable results: `*domain.Server`
- Slice returns for lists (never nil — return empty slice): `make([]domain.Server, 0, len(hzServers))`

## Module Design

**Exports:**
- Expose interfaces in `domain` packages, implementations in `providers` packages
- Use compile-time interface checks: `var _ domain.CatalogProvider = (*HetznerProvider)(nil)`
- Test-only exports are clearly documented: `// Intended for testing.`

**Barrel Files:** Not used. Go packages naturally serve this purpose.

**Provider Registry Pattern:**
- Each resource type (server, dns, sshkey) has its own `providers/registry.go` with `Register()`, `Get()`, `Reset()`, `List()`
- Providers register themselves via `RegisterHetzner()` / `RegisterPorkbun()` / `RegisterCloudflare()`
- Registration happens at startup in `cmd/root.go` via `Execute()`
- Factory pattern: `type Factory func(store auth.Store) (domain.Provider, error)`

## Architecture Patterns

**Domain-First Architecture:**
- Each feature area (`server`, `dns`, `sshkey`) has its own `domain/` package defining interfaces and types
- `providers/` implements the domain interfaces per cloud provider
- `services/` contains business logic that orchestrates provider calls
- `tui/` contains Bubbletea TUI components

**Dependency Injection:**
- Provider implementations receive dependencies through constructors
- Auth store is injected via `auth.Store` interface
- Test code uses mock implementations of interfaces

**Context Propagation:**
- All provider methods accept `context.Context`
- Timeouts applied per-request via `context.WithTimeout(ctx, requestTimeout)`
- Context checked before retry attempts in `retry.Do()`

---

*Convention analysis: 2026-03-20*
