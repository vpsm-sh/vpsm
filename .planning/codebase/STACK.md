# Technology Stack

**Analysis Date:** 2026-03-20

## Languages

**Primary:**
- Go 1.26.0 - Entire codebase (CLI, TUI, API clients, storage)

**Secondary:**
- HTML/CSS - Static marketing site in `web/`

## Runtime

**Environment:**
- Go 1.26.0

**Package Manager:**
- Go modules (`go mod`)
- Lockfile: `go.sum` present

## Frameworks

**Core:**
- `github.com/spf13/cobra` v1.10.2 - CLI command framework (all commands defined in `cmd/`)
- `github.com/charmbracelet/bubbletea` v1.3.10 - Terminal UI framework (interactive wizards)
- `github.com/charmbracelet/huh` v0.8.0 - Form-based TUI prompts
- `github.com/charmbracelet/lipgloss` v1.1.0 - TUI styling and layout
- `github.com/charmbracelet/bubbles` v0.21.1 - TUI component library (spinners, tables, etc.)

**Testing:**
- Go standard `testing` package - All tests
- `github.com/google/go-cmp` v0.7.0 - Deep comparison assertions

**Build/Dev:**
- `make` via `Makefile` - Build, test, lint, release orchestration
- `go vet` - Static analysis
- `staticcheck` (optional) - Additional linting

## Key Dependencies

**Critical:**
- `github.com/hetznercloud/hcloud-go/v2` v2.36.0 - Hetzner Cloud API SDK (server management)
- `github.com/zalando/go-keyring` v0.2.6 - OS keychain integration for credential storage
- `modernc.org/sqlite` v1.45.0 - Pure-Go SQLite driver (action persistence)

**Infrastructure:**
- `golang.org/x/sync` v0.19.0 - Concurrency primitives (errgroup)
- `golang.org/x/term` v0.40.0 - Terminal mode control (secret input)
- `github.com/NimbleMarkets/ntcharts` v0.4.0 - Terminal chart rendering (server metrics)

**TUI Ecosystem (Charm stack):**
- `github.com/charmbracelet/huh/spinner` - Spinner during async operations
- `github.com/charmbracelet/x/ansi` v0.10.1 - ANSI escape sequence handling
- `github.com/charmbracelet/colorprofile` - Terminal color capability detection
- `github.com/lrstanley/bubblezone` - Mouse zone management for TUI

## Configuration

**User Configuration:**
- JSON file at `~/.config/vpsm/config.json` (Linux), `~/Library/Application Support/vpsm/config.json` (macOS), `%AppData%/vpsm/config.json` (Windows)
- Managed by `internal/config/config.go`
- Fields: `default_provider`, `dns_provider`

**Credential Storage:**
- OS keychain via `go-keyring` (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- Service name: `vpsm`
- Keys per provider: single token (Hetzner, Cloudflare) or multi-key (Porkbun: `porkbun-apikey`, `porkbun-secretapikey`)
- Managed by `internal/services/auth/keyring_auth.go`

**Action Persistence:**
- SQLite database at `~/.config/vpsm/vpsm.db`
- Tracks in-flight provider actions for crash recovery
- Managed by `internal/actionstore/repository.go`

**Caching:**
- File-backed JSON cache at OS user cache directory
- Two cache implementations:
  - Simple TTL cache: `internal/cache/cache.go`
  - Stale-while-revalidate cache: `internal/swrcache/cache.go` (5min fresh TTL, 1hr max stale)

**Build:**
- `Makefile` - Primary build interface
- Version metadata embedded via `-ldflags` (`Version`, `Commit`, `BuildTime` in `cmd` package)
- `CGO_ENABLED=0` for static binaries (production builds)

## Build Commands

```bash
make build          # Optimized binary with stripped debug info
make dev            # Quick development build
make test           # Run all tests
make test-verbose   # Tests with -v and -race
make lint           # go vet + staticcheck
make release        # Cross-compile: linux/darwin (amd64+arm64), windows/amd64
make install        # Install to $GOPATH/bin
make clean          # Remove build artifacts
```

## Platform Requirements

**Development:**
- Go 1.26.0+
- make
- Optional: staticcheck for extended linting

**Production:**
- Self-contained static binary (CGO_ENABLED=0)
- Cross-platform: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- OS keychain access required for credential storage

---

*Stack analysis: 2026-03-20*
