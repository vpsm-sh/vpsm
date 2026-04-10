# Codebase Structure

**Analysis Date:** 2026-03-20

## Directory Layout

```
vpsm/
├── main.go                    # Application entry point
├── go.mod                     # Go module definition
├── go.sum                     # Dependency checksums
├── Makefile                   # Build targets
├── cmd/                       # CLI command definitions (Cobra)
│   ├── root.go                # Root command, provider registration, Execute()
│   └── commands/              # Subcommand groups
│       ├── auth/              # Authentication commands (login, status)
│       ├── config/            # Config get/set commands
│       ├── dns/               # DNS record management commands
│       ├── server/            # Server management commands
│       └── sshkey/            # SSH key management commands
├── internal/                  # Private application packages
│   ├── domain/                # Shared domain errors (sentinels)
│   ├── platform/              # Shared platform abstractions
│   │   ├── providers/names/   # Global provider name registry
│   │   └── sshkey/            # Shared SSH key spec type
│   ├── server/                # Server domain vertical slice
│   │   ├── domain/            # Provider interface, entity types, errors
│   │   ├── providers/         # Provider registry + implementations (Hetzner)
│   │   ├── services/action/   # Action tracking + polling service
│   │   └── tui/               # Interactive server TUI (Bubbletea)
│   ├── dns/                   # DNS domain vertical slice
│   │   ├── domain/            # Provider interface, Record/Domain types, errors
│   │   ├── providers/         # Provider registry + implementations (Porkbun, Cloudflare)
│   │   ├── services/          # DNS service layer (validation, caching)
│   │   └── tui/               # Interactive DNS TUI (Bubbletea)
│   ├── sshkey/                # SSH key domain vertical slice
│   │   ├── domain/            # Provider interface
│   │   ├── providers/         # Provider registry + implementations (Hetzner)
│   │   └── tui/               # Interactive SSH key TUI
│   ├── services/              # Shared services
│   │   ├── auth/              # Auth store interface + keyring implementation
│   │   └── serverprefs/       # Server preferences service
│   ├── config/                # User config (JSON file)
│   ├── actionstore/           # Action persistence (SQLite)
│   ├── serverprefs/           # Server prefs persistence (SQLite)
│   ├── swrcache/              # Stale-while-revalidate cache (file-backed)
│   ├── cache/                 # Generic cache utilities
│   ├── retry/                 # Retry with exponential backoff
│   ├── tui/                   # Shared TUI components
│   │   ├── components/        # Footer, status bar
│   │   └── styles/            # Colors, lipgloss styles
│   ├── util/                  # String helpers, validators
│   ├── sshkeys/               # SSH key file utilities
│   └── volume/                # Volume-related types (placeholder)
├── web/                       # Web assets (images, fonts)
│   └── public/
│       ├── images/            # Screenshots, favicons
│       └── fonts/             # Custom fonts
├── docs/                      # Documentation
│   ├── images/                # Doc images
│   └── reference/server/      # Server command reference
├── build/                     # Build artifacts/scripts
├── .github/workflows/         # CI/CD workflows
├── .planning/                 # Planning documents
│   └── codebase/              # Codebase analysis (this file)
└── tmp/                       # Temporary files (not committed)
```

## Directory Purposes

**cmd/commands/server/:**
- Purpose: All `vpsm server` subcommands
- Contains: One file per subcommand (list.go, create.go, delete.go, show.go, ssh.go, start.go, stop.go, metrics.go, actions.go) plus shared output helpers (output.go)
- Key files: `server.go` (parent command + provider resolution), `list.go` (default command, TUI or table/JSON), `ssh.go` (shell-out to SSH)

**cmd/commands/dns/:**
- Purpose: All `vpsm dns` subcommands
- Contains: `dns.go` (parent command + DNS provider resolution + service factory), `list.go`, `create.go`, `update.go`, `delete.go`, `domains.go`
- Key files: `dns.go` (wires provider to service layer via `newDNSService()`)

**internal/server/domain/:**
- Purpose: Server domain types and provider contract
- Contains: `provider.go` (Provider, CatalogProvider, ActionPoller, MetricsProvider interfaces), `catalog.go` (Location, ServerTypeSpec, ImageSpec, SSHKeySpec structs), `action.go` (ActionStatus), `metrics.go` (MetricType, ServerMetrics), `errors.go`
- Key files: `provider.go` defines the full server provider interface hierarchy

**internal/dns/domain/:**
- Purpose: DNS domain types and provider contract
- Contains: `provider.go` (Provider interface), `record.go` (Record, Domain, RecordType), `create_opts.go`, `errors.go`
- Key files: `provider.go` is the single interface all DNS providers must implement

**internal/*/providers/:**
- Purpose: Provider factory registries and concrete implementations
- Contains: `registry.go` (Register/Get/List/Reset functions), provider implementation files
- Key files: `registry.go` in each domain is the factory map; implementation files named by provider (e.g. `hetzner_catalog.go`, `porkbun.go`, `cloudflare.go`)

**internal/services/auth/:**
- Purpose: Authentication abstraction
- Contains: `auth.go` (Store interface, DefaultStore factory), `keyring_auth.go` (KeyringStore), `mock_store.go` (testing)
- Key files: `auth.go` defines the `Store` interface used everywhere

**internal/actionstore/:**
- Purpose: SQLite persistence for in-flight server actions
- Contains: `repository.go` (ActionRepository interface + SQLiteRepository), `record.go` (ActionRecord struct)
- Key files: `repository.go` handles migrations, CRUD, cleanup

**internal/swrcache/:**
- Purpose: Stale-while-revalidate caching with file-backed JSON
- Contains: `cache.go` (Cache struct, generic GetOrFetch, Invalidate)
- Key files: `cache.go` is the complete implementation

## Key File Locations

**Entry Points:**
- `main.go`: Application entry, calls `cmd.Execute()`
- `cmd/root.go`: Cobra root command, provider registration, command tree assembly

**Configuration:**
- `internal/config/config.go`: User config load/save (`~/.config/vpsm/config.json`)
- `internal/config/keys.go`: Config key registry (default-provider, dns-provider)
- `go.mod`: Module path `nathanbeddoewebdev/vpsm`, Go version, dependencies
- `Makefile`: Build targets

**Core Logic:**
- `internal/server/domain/provider.go`: Server provider interface hierarchy
- `internal/dns/domain/provider.go`: DNS provider interface
- `internal/sshkey/domain/provider.go`: SSH key provider interface
- `internal/dns/services/service.go`: DNS service layer (validation + caching wrapper)
- `internal/server/services/action/service.go`: Action tracking + polling service
- `internal/services/auth/auth.go`: Auth store interface

**Provider Implementations:**
- `internal/server/providers/hetzner_catalog.go`: Hetzner server provider
- `internal/dns/providers/porkbun.go`: Porkbun DNS provider
- `internal/dns/providers/cloudflare.go`: Cloudflare DNS provider
- `internal/sshkey/providers/hetzner.go`: Hetzner SSH key provider

**TUI:**
- `internal/server/tui/create.go`: Server creation wizard (huh forms)
- `internal/dns/tui/dns_app.go`: DNS TUI app shell (navigation messages)
- `internal/dns/tui/dns_domain_list.go`: DNS domain list view
- `internal/dns/tui/dns_record_list.go`: DNS record list view
- `internal/tui/styles/styles.go`: Shared lipgloss styles
- `internal/tui/components/footer.go`: Shared TUI footer component

**Testing:**
- `internal/config/config_test.go`, `internal/config/keys_test.go`
- `internal/retry/retry_test.go`
- `internal/cache/cache_test.go`
- `internal/server/services/action/service_test.go`
- `internal/dns/services/service_test.go`
- `internal/dns/providers/cloudflare_test.go`, `internal/dns/providers/porkbun_test.go`
- `cmd/commands/server/*_test.go` (list, show, delete, start, stop, ssh, actions, metrics)

## Naming Conventions

**Files:**
- lowercase with underscores for multi-word: `hetzner_catalog.go`, `mock_store.go`, `dns_app.go`
- one file per subcommand in cmd layer: `list.go`, `create.go`, `delete.go`
- test files co-located with source: `service_test.go` next to `service.go`

**Directories:**
- lowercase, singular for domain concepts: `server`, `dns`, `sshkey`, `domain`, `tui`
- lowercase, plural for collections: `providers`, `services`, `components`, `styles`

**Packages:**
- Match directory name
- Import aliases used to resolve collisions: `cfgcmd`, `dnsproviders`, `serverproviders`, `prefssvc`

## Where to Add New Code

**New Cloud Provider (e.g. DigitalOcean servers):**
- Create `internal/server/providers/digitalocean.go` implementing `domain.Provider` (+ optional extended interfaces)
- Add `RegisterDigitalOcean()` function that calls `Register("digitalocean", factory)`
- Call `RegisterDigitalOcean()` in `cmd/root.go:Execute()`
- Follow the same pattern for sshkey domain if needed

**New DNS Provider:**
- Create `internal/dns/providers/<name>.go` implementing `dns/domain.Provider`
- Add `Register<Name>()` function
- Call it in `cmd/root.go:Execute()`

**New Server Subcommand:**
- Add `cmd/commands/server/<verb>.go` with `<Verb>Command() *cobra.Command`
- Register in `cmd/commands/server/server.go:NewCommand()` via `cmd.AddCommand()`
- Add corresponding test file `cmd/commands/server/<verb>_test.go`

**New DNS Subcommand:**
- Add `cmd/commands/dns/<verb>.go`
- Register in `cmd/commands/dns/dns.go:NewCommand()`

**New Domain Vertical (e.g. volumes, firewalls):**
- Create `internal/<domain>/domain/provider.go` (interface)
- Create `internal/<domain>/providers/registry.go` (factory registry)
- Create `internal/<domain>/providers/<impl>.go` (implementation)
- Create `cmd/commands/<domain>/<domain>.go` (parent command)
- Optionally add `internal/<domain>/services/` and `internal/<domain>/tui/`
- Wire into `cmd/root.go`

**New Config Key:**
- Add field to `Config` struct in `internal/config/config.go`
- Add `KeySpec` entry to `Keys` slice in `internal/config/keys.go`

**Shared TUI Components:**
- Add to `internal/tui/components/`

**Shared Utilities:**
- Add to `internal/util/`

## Special Directories

**web/:**
- Purpose: Static web assets (screenshots, favicons, fonts) for documentation/website
- Generated: No
- Committed: Yes

**build/:**
- Purpose: Build scripts and artifacts
- Generated: Partially (binary artifacts)
- Committed: Scripts yes, artifacts no

**tmp/:**
- Purpose: Temporary files
- Generated: Yes
- Committed: No (in .gitignore)

**.planning/codebase/:**
- Purpose: Codebase analysis documents for AI-assisted development
- Generated: Yes (by mapping tools)
- Committed: Yes

---

*Structure analysis: 2026-03-20*
