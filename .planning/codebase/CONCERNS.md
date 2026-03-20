# Codebase Concerns

**Analysis Date:** 2026-03-20

## Tech Debt

**Hardcoded Application Version:**
- Issue: The Hetzner client is initialized with a hardcoded version string `"0.1.0"` rather than pulling from a build variable or constant.
- Files: `internal/server/providers/hetzner.go` (line 41)
- Impact: Version reported to the Hetzner API never updates, making debugging API issues harder and misrepresenting the client to the provider.
- Fix approach: Define a package-level `Version` variable set via `-ldflags` at build time, and reference it in the `hcloud.WithApplication` call.

**Silenced Errors Throughout Codebase (`_ =` pattern):**
- Issue: Many error returns are intentionally discarded with `_ =`, particularly for cache writes, repo saves, and cache invalidation. While individually defensible as "best-effort" operations, the pattern is widespread and makes it impossible to diagnose silent failures.
- Files:
  - `internal/services/serverprefs/service.go` (line 48) - `_ = s.repo.Save(prefs)`
  - `internal/server/services/action/service.go` (lines 87, 104, 150, 159) - repo saves and cleanup
  - `internal/dns/services/service.go` (lines 105, 141, 157) - cache invalidation
  - `internal/server/providers/hetzner_catalog.go` (lines 46, 80, 116) - cache writes
  - `internal/swrcache/cache.go` (lines 137, 149, 190, 194) - cache writes/deletes
  - `internal/cache/cache.go` (lines 42, 80, 84) - file operations
- Impact: Persistent failures (e.g., disk full, permissions) go completely undetected. Users see stale data or lost preferences with no indication of why.
- Fix approach: Add structured logging (e.g., `slog`) so discarded errors are at least logged at debug/warn level, allowing diagnosis without changing control flow.

**Duplicate Provider Registry Pattern:**
- Issue: Three nearly identical provider registry implementations exist for server, DNS, and SSH key domains. Each has its own `Register`, `Get`, `List`, `Reset` functions with the same mutex-guarded map pattern.
- Files:
  - `internal/server/providers/registry.go`
  - `internal/dns/providers/registry.go`
  - `internal/sshkey/providers/registry.go`
- Impact: Triple maintenance burden for any change to the registry pattern. Bug fixes or improvements must be applied in three places.
- Fix approach: Extract a generic `Registry[T]` type parameterized by the provider interface, and have each domain instantiate it.

**Re-exported Sentinel Errors:**
- Issue: Both `internal/server/domain/errors.go` and `internal/dns/domain/errors.go` re-export the same sentinel errors from `internal/domain/errors.go` via variable assignment. This creates aliases (`dns.ErrNotFound == domain.ErrNotFound`) which works for `errors.Is` but is an unusual pattern that could confuse contributors.
- Files:
  - `internal/server/domain/errors.go`
  - `internal/dns/domain/errors.go`
  - `internal/domain/errors.go`
- Impact: Low functional risk since the aliases work correctly with `errors.Is`. Primarily a maintainability concern.
- Fix approach: Have consumers import `internal/domain` directly, or document the aliasing pattern in each file.

**Empty Placeholder Directories:**
- Issue: `internal/volume/` contains only a README placeholder, and `internal/store/` is completely empty. These suggest planned features that were never implemented.
- Files: `internal/volume/README.md`, `internal/store/` (empty)
- Impact: Clutters the codebase structure and may confuse new contributors about the project's actual scope.
- Fix approach: Either implement the planned features or remove the placeholder directories until they are needed.

**Unused Variable in SSH Key Add:**
- Issue: `RunSSHKeyAddAccessible` discards the `providerName` parameter with `_ = providerName`.
- Files: `internal/sshkey/tui/ssh_key_add.go` (line 159)
- Impact: Minor. The provider name is not used in the accessible flow, suggesting an incomplete implementation.
- Fix approach: Either use `providerName` in the UI (e.g., as a title/label) or remove the parameter.

## Security Considerations

**Config File Permissions:**
- Risk: Configuration file (`config.json`) is written with `0o644` (world-readable). While secrets are stored in the OS keyring (not in the config file), the config reveals which providers are configured, which could be a minor information leak.
- Files: `internal/config/config.go` (line 113)
- Current mitigation: Secrets are properly stored in the OS keyring via `go-keyring`. Config file only contains provider names.
- Recommendations: Consider using `0o600` for the config file to follow principle of least privilege.

**No Request Body Size Limits on HTTP Responses:**
- Risk: The Cloudflare and Porkbun providers decode JSON responses directly from `resp.Body` without limiting the response size. A malicious or malfunctioning API could return an extremely large response causing memory exhaustion.
- Files:
  - `internal/dns/providers/cloudflare.go` (line 212) - `json.NewDecoder(resp.Body).Decode(out)`
  - `internal/dns/providers/porkbun.go` (line 119) - `json.NewDecoder(resp.Body).Decode(out)`
- Current mitigation: HTTP client has a 30-second timeout which provides some protection.
- Recommendations: Wrap `resp.Body` in an `io.LimitReader` (e.g., 10MB limit) before decoding.

**Global Mutable State for Test Path Overrides:**
- Risk: `pathOverride` variables in `internal/config/config.go` and `internal/actionstore/repository.go` use package-level mutable state for test path overrides. If tests run in parallel and forget to call `ResetPath()`, they can interfere with each other.
- Files:
  - `internal/config/config.go` (lines 23-29)
  - `internal/actionstore/repository.go` (lines 28-34)
- Current mitigation: Tests appear to manage this correctly, but the pattern is fragile.
- Recommendations: Use `t.Setenv` or pass paths via function parameters instead of global state.

## Performance Bottlenecks

**Cloudflare Zone Lookup on Every Record Operation:**
- Problem: Every DNS record operation (list, create, update, delete) calls `getZoneID` which makes a separate API request to resolve the domain name to a Cloudflare zone ID. There is no caching of zone ID lookups.
- Files: `internal/dns/providers/cloudflare.go` (lines 222-237, 280, and called from every record method)
- Cause: Zone IDs are ephemeral within a single `CloudflareProvider` instance. Unlike the Hetzner provider which has a `cache` field, the Cloudflare provider has no caching layer.
- Improvement path: Cache zone ID lookups in a map on the provider struct (zone IDs are stable). The DNS service layer already has SWR caching for record lists, but the underlying zone lookup is redundant on every call.

**No Retry Logic for DNS Providers:**
- Problem: The Hetzner server provider uses `internal/retry` for all API calls with exponential backoff. The Cloudflare and Porkbun DNS providers have no retry logic at all.
- Files:
  - `internal/dns/providers/cloudflare.go` - no retry usage
  - `internal/dns/providers/porkbun.go` - no retry usage
  - `internal/server/providers/hetzner.go` - uses `retry.Do` consistently
- Cause: DNS providers were added later and may not have needed retries initially.
- Improvement path: Apply the same `retry.Do` pattern from the server provider to DNS provider HTTP calls.

## Fragile Areas

**TUI Models with Type Assertions:**
- Files:
  - `internal/server/tui/server_app.go` (lines 362-391)
- Why fragile: `updateChildDirect` casts `tea.Model` results back to concrete types (e.g., `updated.(serverListModel)`) which will panic if a child model's `Update` method ever returns the wrong type. This is standard Bubbletea practice but creates runtime crash risk.
- Safe modification: When modifying any child model's `Update` method, always ensure it returns the same concrete type. Consider adding recovery with `defer` or using comma-ok assertions.
- Test coverage: `internal/server/tui/create_test.go` and `internal/server/tui/delete_test.go` exist but test coverage for the app model's navigation and type assertion paths is limited.

**Large TUI Files:**
- Files:
  - `internal/server/tui/server_app.go` (956 lines)
  - `internal/server/tui/server_show.go` (950 lines)
  - `internal/server/tui/server_create.go` (950 lines)
  - `internal/server/tui/ops_overlay.go` (813 lines)
  - `internal/sshkey/tui/ssh_key_add.go` (740 lines)
- Why fragile: These files are large and interleave state machine logic with rendering. Changes to one state transition can have subtle effects on others. The create wizard alone has 7 steps (`stepLoading` through `stepConfirm`), each with their own keyboard handling.
- Safe modification: When adding steps or views, follow the existing pattern strictly. Test keyboard navigation end-to-end. The overlay model is particularly complex as it manages concurrent operations.
- Test coverage: Only `create_test.go` (260 lines) and `delete_test.go` cover TUI logic. The 950-line `server_show.go` and 956-line `server_app.go` have no dedicated tests.

**Deprecated `net.Error.Temporary()` Usage:**
- Files: `internal/retry/retry.go` (line 79)
- Why fragile: `netErr.Temporary()` is deprecated in Go and always returns `false` in newer versions. The retry predicate relies on it for deciding whether to retry network errors.
- Safe modification: Remove the `Temporary()` check. The `Timeout()` check on the same line and the provider-specific retryable predicates (like `isHetznerRetryable`) already cover the important cases.
- Test coverage: `internal/retry/retry_test.go` exists but does not test the `Temporary()` branch specifically.

## Test Coverage Gaps

**TUI Layer Entirely Untested (DNS and Shared):**
- What's not tested: The entire DNS TUI (`internal/dns/tui/`) with 7 model files totaling ~2,500 lines, and the shared TUI package (`internal/tui/`) with auth and config views, have zero test files.
- Files:
  - `internal/dns/tui/dns_app.go` (371 lines)
  - `internal/dns/tui/dns_record_list.go` (447 lines)
  - `internal/dns/tui/dns_record_create.go` (443 lines)
  - `internal/dns/tui/dns_record_edit.go` (346 lines)
  - `internal/dns/tui/dns_record_delete.go` (196 lines)
  - `internal/dns/tui/dns_domain_list.go` (339 lines)
  - `internal/dns/tui/dns_record_show.go`
  - `internal/tui/auth_login.go` (176 lines)
  - `internal/tui/config_view.go` (264 lines)
  - `internal/tui/auth_status.go`
  - `internal/tui/components/` (header, footer, statusbar, metrics_chart)
- Risk: Regressions in interactive flows go completely undetected. The DNS TUI handles user input, state transitions, and API calls.
- Priority: Medium - TUI testing is notoriously difficult with Bubbletea, but the `internal/server/tui/create_test.go` shows it is feasible in this codebase.

**Auth Package Not Tested:**
- What's not tested: `internal/services/auth/auth.go` and `internal/services/auth/keyring_auth.go` have no test files. The `mock_store.go` exists as a test helper but is only used by other packages' tests.
- Files:
  - `internal/services/auth/auth.go`
  - `internal/services/auth/keyring_auth.go`
- Risk: Auth token storage/retrieval errors could go unnoticed. The keyring integration is platform-specific (macOS Keychain, Linux Secret Service, Windows Credential Manager).
- Priority: Medium - the `KeyringStore` is thin wrapper around `go-keyring`, but the error mapping logic (e.g., converting `keyring.ErrNotFound` to `ErrTokenNotFound`) should be verified.

**SSH Key Provider Not Tested:**
- What's not tested: `internal/sshkey/providers/hetzner.go` has no test file.
- Files: `internal/sshkey/providers/hetzner.go`
- Risk: SSH key upload to Hetzner could break silently.
- Priority: Low - the implementation likely mirrors the well-tested server provider pattern.

**Server Services Layer Partially Tested:**
- What's not tested: `internal/server/services/hcloud.go` (246 lines) has no test file. This is the service layer that wraps the Hetzner client for server creation, start, stop, and action polling.
- Files: `internal/server/services/hcloud.go`
- Risk: This is a critical path for all server operations. The action service (`internal/server/services/action/service.go`) is well-tested, but the underlying `HCloudService` it delegates to is not.
- Priority: Medium - the provider tests in `internal/server/providers/hetzner_test.go` partially cover this indirectly.

**No Integration or E2E Tests:**
- What's not tested: There are no integration tests that exercise the full command path (cobra command -> service -> provider). All existing tests are unit tests with mocked providers.
- Risk: Issues in wiring (e.g., flag parsing, provider resolution, error display) are only caught manually.
- Priority: Low - the CLI is interactive, making automated E2E testing challenging.

## Dependencies at Risk

**`netErr.Temporary()` Deprecation:**
- Risk: The `net.Error.Temporary()` method is deprecated in Go stdlib. It always returns `false` in newer Go versions, meaning the retry predicate's `Temporary()` check is dead code.
- Files: `internal/retry/retry.go` (line 79)
- Impact: No functional impact currently since `Timeout()` on the same line handles the important case. But relying on deprecated API signals technical debt.
- Migration plan: Remove the `|| netErr.Temporary()` clause.

**Node.js `.gitignore` Remnants:**
- Risk: The `.gitignore` file contains Node.js-specific entries (`node_modules`, `.eslintcache`, `*.tsbuildinfo`) suggesting the project started as or was templated from a Node.js project. This is cosmetic but misleading.
- Files: `.gitignore`
- Impact: No functional impact. Minor confusion for contributors.
- Migration plan: Clean up `.gitignore` to reflect the Go-only backend. Keep web-related entries if `web/` directory is actively used.

## Missing Critical Features

**No Logging Framework:**
- Problem: The entire codebase has no structured logging. Errors are either returned up the call stack, silenced with `_ =`, or written to `io.Writer` (typically stderr) via `fmt.Fprintf`.
- Blocks: Debugging production issues, understanding cache behavior, tracking API call patterns, diagnosing "best-effort" operation failures.
- Files: All files - no imports of `log`, `slog`, or any logging library.

**No Graceful Context Propagation in DNS Commands:**
- Problem: DNS command handlers use `context.Background()` directly, meaning they cannot be interrupted with Ctrl+C during API calls. Server commands correctly use `signal.NotifyContext`.
- Files:
  - `cmd/commands/dns/domains.go` (line 45) - `context.Background()`
  - `cmd/commands/dns/create.go` (line 54) - `context.Background()`
  - `cmd/commands/dns/delete.go` (line 32) - `context.Background()`
  - `cmd/commands/dns/list.go` (line 54) - `context.Background()`
  - `cmd/commands/dns/update.go` (line 59) - `context.Background()`
  - Compare: `cmd/commands/server/start.go` (line 55) - uses `signal.NotifyContext`
- Blocks: Users cannot cancel long-running DNS operations. With no retry logic in DNS providers, a network issue means waiting for the full 30-second timeout.

---

*Concerns audit: 2026-03-20*
