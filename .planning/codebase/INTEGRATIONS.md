# External Integrations

**Analysis Date:** 2026-03-20

## APIs & External Services

**Cloud Server Management:**
- Hetzner Cloud API - Server CRUD, SSH key management, server metrics, action polling
  - SDK: `github.com/hetznercloud/hcloud-go/v2` (official Go SDK)
  - Auth: API token stored in OS keychain under key `hetzner`
  - Provider: `internal/server/providers/hetzner.go`
  - Capabilities: `Provider`, `CatalogProvider`, `SSHKeyManager`, `ActionPoller`, `MetricsProvider`
  - Timeout: 30s per request
  - Retry: 3 attempts, 500ms base delay, 5s max delay (exponential backoff with jitter)
  - User-Agent: `vpsm/0.1.0`

**DNS Management:**
- Porkbun API v3 - Domain listing, DNS record CRUD
  - SDK: Direct HTTP client (no SDK, raw `net/http`)
  - Base URL: `https://api.porkbun.com/api/json/v3`
  - Auth: Two keychain entries - `porkbun-apikey` and `porkbun-secretapikey`
  - Provider: `internal/dns/providers/porkbun.go`
  - Timeout: 30s

- Cloudflare API v4 - Domain listing, DNS record CRUD, domain search
  - SDK: Direct HTTP client (no SDK, intentionally lightweight)
  - Base URL: `https://api.cloudflare.com/client/v4`
  - Auth: Scoped Account API Token (not Global API Key) stored under key `cloudflare`
  - Required permissions: Zone:Read, DNS:Edit; Account:Read + Registrar for domain search
  - Account ID is auto-discovered from the token on first search call and cached per instance.
  - Provider: `internal/dns/providers/cloudflare.go`
  - Timeout: 30s

**SSH Key Management:**
- Hetzner Cloud API - SSH key creation and listing (reuses server provider connection)
  - Provider: `internal/sshkey/providers/hetzner.go`

## Provider Registry System

All providers use a factory-based registry pattern. Providers are registered at startup in `cmd/root.go`:

```go
serverproviders.RegisterHetzner()
sshkeyproviders.RegisterHetzner()
dnsproviders.RegisterPorkbun()
dnsproviders.RegisterCloudflare()
```

**Server Provider Registry:** `internal/server/providers/registry.go`
**DNS Provider Registry:** `internal/dns/providers/registry.go`
**SSH Key Provider Registry:** `internal/sshkey/providers/registry.go`

Each registry maps a normalized provider name to a `Factory` function that takes an `auth.Store` and returns the domain provider interface.

## Data Storage

**Databases:**
- SQLite (pure Go, `modernc.org/sqlite`)
  - Location: `~/.config/vpsm/vpsm.db`
  - Purpose: Persist in-flight actions for crash recovery
  - Client: `database/sql` with `modernc.org/sqlite` driver
  - Schema: `internal/actionstore/repository.go`

**File Storage:**
- Local filesystem only
  - Config: `~/.config/vpsm/config.json` (`internal/config/config.go`)
  - Cache: OS user cache directory (`internal/cache/cache.go`, `internal/swrcache/cache.go`)
    - JSON-serialized cache entries with TTL-based expiry
    - Used for provider catalog data (server types, locations, images)

**Caching:**
- File-backed JSON cache (two implementations):
  - Simple TTL: `internal/cache/cache.go` - used by Hetzner provider for catalog data (1hr TTL)
  - SWR (stale-while-revalidate): `internal/swrcache/cache.go` - 5min fresh, 1hr max stale

## Authentication & Identity

**Auth Provider:**
- Custom, OS keychain-based
  - Implementation: `internal/services/auth/keyring_auth.go` using `github.com/zalando/go-keyring`
  - Interface: `internal/services/auth/auth.go` defines `Store` interface (`GetToken`, `SetToken`, `DeleteToken`)
  - Mock: `internal/services/auth/mock_store.go` for testing
  - Service name: `vpsm`
  - Each provider's credentials stored under normalized provider name (e.g., `hetzner`, `cloudflare`, `porkbun-apikey`)

**Credential Specs:**
- Defined in `internal/platform/providers/credentials.go`
- Three registered providers:
  - Hetzner: single API token
  - Porkbun: API key + secret API key (two keychain entries)
  - Cloudflare: single Account API Token

**Login Flow:**
- `vpsm auth login <provider>` prompts for credentials based on the provider's `CredentialSpec`
- Tokens stored in OS keychain, retrieved at provider construction time

## Monitoring & Observability

**Error Tracking:**
- None (no external error tracking service)

**Logs:**
- No structured logging framework
- Errors returned up the call stack and displayed to user via TUI or stderr

**Metrics:**
- `prometheus/client_golang` is an indirect dependency (pulled in by hcloud-go), not used directly by vpsm

## CI/CD & Deployment

**Hosting:**
- GitHub Releases - Binary distribution
- GitHub Pages - Static marketing site (`web/` directory)

**CI Pipeline:**
- GitHub Actions
  - `.github/workflows/release.yml` - Triggered on `v*` tags: lint, test, cross-compile, publish release with checksums
  - `.github/workflows/static.yml` - Deploy `web/` to GitHub Pages on push to main (when `web/**` changes)

**Release Process:**
1. `make tag TAG=v1.2.3` - Creates and pushes annotated git tag
2. GitHub Actions triggers release workflow
3. Cross-compiles for 5 platforms (linux/darwin amd64+arm64, windows/amd64)
4. Publishes binaries + SHA256 checksums as GitHub Release assets

## Environment Configuration

**Required credentials (stored in OS keychain, not env vars):**
- Hetzner: API token (for server/SSH key operations)
- Porkbun: API key + secret API key (for DNS operations)
- Cloudflare: Account API token with Zone:Read + DNS:Edit (for DNS operations)

**No `.env` files:** This application does not use environment variables for configuration. All secrets are stored in the OS keychain and all config is in `~/.config/vpsm/config.json`.

## Webhooks & Callbacks

**Incoming:**
- None

**Outgoing:**
- None

## Domain Interfaces

**Server Provider Interface:** `internal/server/domain/provider.go`
- `Provider` - Core CRUD + start/stop
- `CatalogProvider` - List locations, server types, images, SSH keys
- `SSHKeyManager` - Create SSH keys
- `ActionPoller` - Poll async action status
- `MetricsProvider` - Fetch time-series metrics (CPU, disk, network)

**DNS Provider Interface:** `internal/dns/domain/provider.go`
- `Provider` - List domains, CRUD DNS records

**Adding a new provider:**
1. Create provider implementation in `internal/{server,dns,sshkey}/providers/`
2. Add credential spec to `internal/platform/providers/credentials.go`
3. Register factory in `cmd/root.go` Execute function

---

*Integration audit: 2026-03-20*
