# Testing Patterns

**Analysis Date:** 2026-03-20

## Test Framework

**Runner:**
- Go standard `testing` package
- No third-party test runner

**Assertion Library:**
- Standard `testing.T` methods: `t.Fatal()`, `t.Fatalf()`, `t.Error()`, `t.Errorf()`
- `github.com/google/go-cmp/cmp` for struct comparison via `cmp.Diff()`
- No testify or other assertion libraries

**Run Commands:**
```bash
make test              # Run all tests (go test ./... -count=1)
make test-verbose      # Run tests with -v and -race detector
go test ./... -count=1 # Direct invocation
```

## Test File Organization

**Location:**
- Co-located with source files in the same package (same directory)
- Test files use `_test.go` suffix: `hetzner.go` -> `hetzner_test.go`

**Naming:**
- File: `<source_file>_test.go`
- Test function: `Test<Function>_<Scenario>` (e.g., `TestListServers_HappyPath`, `TestDeleteServer_NotFound`)

**Structure:**
```
internal/server/providers/
├── hetzner.go
├── hetzner_test.go           # Tests for core server operations
├── hetzner_catalog.go
├── hetzner_catalog_test.go   # Tests for catalog operations
├── hetzner_metrics.go
├── hetzner_metrics_test.go   # Tests for metrics
└── registry.go
```

## Test Structure

**Suite Organization:**
```go
// Test helpers at the top of file, prefixed with section comments
// --- Test helpers ---

func newTestHetznerProvider(t *testing.T, serverURL string, token string) *HetznerProvider {
    t.Helper()
    // setup...
}

// --- Section Name tests ---

func TestListServers_HappyPath(t *testing.T) {
    // Arrange
    mock := &mockProvider{...}

    // Act
    result, err := svc.ListServers(ctx)

    // Assert
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if diff := cmp.Diff(want, got); diff != "" {
        t.Errorf("mismatch (-want +got):\n%s", diff)
    }
}
```

**Patterns:**
- Use `t.Helper()` on all helper functions
- Use `t.Cleanup()` for teardown (not `defer` in test body for shared resources)
- Use `t.TempDir()` for filesystem-dependent tests
- Test naming: `Test<Unit>_<Scenario>` with underscores separating unit from scenario
- Section comments group related tests: `// --- ListServers tests ---`, `// --- DeleteServer tests ---`

## Mocking

**Framework:** Hand-rolled mocks implementing domain interfaces. No mock generation tools.

**Mock Provider Pattern (for domain.Provider):**
```go
type mockProvider struct {
    displayName string
    servers     []domain.Server
    listErr     error
}

func (m *mockProvider) GetDisplayName() string { return m.displayName }
func (m *mockProvider) ListServers(_ context.Context) ([]domain.Server, error) {
    return m.servers, m.listErr
}
// ... stub remaining interface methods with fmt.Errorf("not implemented")
```

**Mock Store Pattern (for auth.Store):**
```go
// Located at: internal/services/auth/mock_store.go
store := auth.NewMockStore()
store.SetToken("hetzner", "test-token")
```

**Mock Repository Pattern (for persistence):**
```go
type mockRepository struct {
    records    []actionstore.ActionRecord
    saveErr    error
    listPendingErr error
}
// Implement interface methods, returning configured errors/data
```

**What to Mock:**
- External API providers (Hetzner, Porkbun, Cloudflare) — either via mock struct or `httptest.Server`
- Auth store — use `auth.NewMockStore()` from `internal/services/auth/mock_store.go`
- Persistence repositories — hand-rolled mock structs
- Global registries — use `providers.Reset()` with `t.Cleanup()`

**What NOT to Mock:**
- Domain types and value objects
- Internal utility functions (`retry`, `cache`, `config`)
- Standard library functionality

## HTTP API Testing

**Pattern:** Use `net/http/httptest.Server` to simulate external APIs.

```go
func TestListServers_HappyPath(t *testing.T) {
    // Build JSON response matching the provider's API shape
    response := map[string]any{
        "servers": []any{testServerJSON(42, "web-server", "running", ...)},
    }

    // Create test HTTP server
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Optionally assert request details
        if r.URL.Path != "/servers" {
            t.Errorf("expected path /servers, got %s", r.URL.Path)
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
    }))
    t.Cleanup(srv.Close)

    // Point provider at test server
    provider := newTestHetznerProvider(t, srv.URL, "test-token")
    servers, err := provider.ListServers(context.Background())
    // ... assertions
}
```

**JSON Builder Helpers:**
- Each provider test file defines helper functions to build API-shaped JSON:
  - `testLocationJSON()`, `testServerTypeJSON()`, `testImageJSON()`, `testServerJSON()` in `internal/server/providers/hetzner_test.go`
  - `cfSuccessEnvelope()`, `cfErrorEnvelope()`, `testCFZoneJSON()`, `testCFRecordJSON()` in `internal/dns/providers/cloudflare_test.go`
- Helpers return `map[string]any` for easy composition and modification before encoding

**Reusable Test API Server:**
```go
// newTestAPI spins up an httptest.Server that returns the given response as JSON.
func newTestAPI(t *testing.T, response any) *httptest.Server {
    t.Helper()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
    }))
    t.Cleanup(srv.Close)
    return srv
}
```

## CLI Command Testing

**Pattern:** Use Cobra's built-in test support with buffer capture.

```go
func execList(t *testing.T, providerName string) (stdout, stderr string) {
    t.Helper()
    var outBuf, errBuf bytes.Buffer
    cmd := NewCommand()
    cmd.SetOut(&outBuf)
    cmd.SetErr(&errBuf)
    cmd.SetArgs([]string{"list", "--provider", providerName})
    cmd.Execute()
    return outBuf.String(), errBuf.String()
}
```

**Assertion Helpers:**
```go
func assertContainsAll(t *testing.T, output string, label string, expected []string) {
    t.Helper()
    for _, want := range expected {
        if !strings.Contains(output, want) {
            t.Errorf("expected %q in %s output:\n%s", want, label, output)
        }
    }
}
```

**Registry Isolation:**
```go
func registerMockProvider(t *testing.T, name string, mock *mockProvider) {
    t.Helper()
    providers.Reset()
    t.Cleanup(func() { providers.Reset() })
    providers.Register(name, func(store auth.Store) (domain.Provider, error) {
        return mock, nil
    })
}
```

## Table-Driven Tests

**Pattern:** Used for pure functions with multiple input/output combinations.

```go
func TestNormalizeDomain(t *testing.T) {
    cases := []struct {
        input string
        want  string
    }{
        {"example.com", "example.com"},
        {"EXAMPLE.COM", "example.com"},
        {"example.com.", "example.com"},
        {"  example.com.  ", "example.com"},
        {"", ""},
    }

    for _, c := range cases {
        got := normalizeDomain(c.input)
        if got != c.want {
            t.Errorf("normalizeDomain(%q) = %q, want %q", c.input, got, c.want)
        }
    }
}
```

**Convention:** Use anonymous struct slices named `cases`. Iterate with `for _, c := range cases`.

## Fixtures and Factories

**Test Data:**
- Built inline using helper functions (no separate fixture files)
- JSON builder helpers produce `map[string]any` that mirror real API responses
- Domain objects constructed with struct literals in tests

**Location:**
- Test helpers and mock types live at the top of `_test.go` files
- `auth.MockStore` is the only mock in a production file: `internal/services/auth/mock_store.go`
- No shared `testdata/` directories or fixture files

## Coverage

**Requirements:** None enforced. No coverage thresholds configured.

**View Coverage:**
```bash
go test ./... -cover              # Summary per package
go test ./... -coverprofile=c.out # Generate profile
go tool cover -html=c.out        # View in browser
```

## Test Types

**Unit Tests:**
- All tests are unit-level
- Test individual functions, methods, and command execution
- Use mocks/stubs for external dependencies
- Located in `_test.go` files co-located with source

**Integration Tests:**
- Not present. External API calls are always mocked with `httptest.Server`

**E2E Tests:**
- Not present

## Common Patterns

**Async Testing:**
```go
// Used in swrcache tests for background revalidation
called := make(chan struct{}, 1)
fetch := func(ctx context.Context) (string, error) {
    called <- struct{}{}
    return "fresh", nil
}

select {
case <-called:
    // background fetch happened
case <-time.After(time.Second):
    t.Fatal("expected background revalidation")
}
```

**Error Testing:**
```go
// Pattern 1: Check error is non-nil
_, err := provider.GetServer(ctx, "not-a-number")
if err == nil {
    t.Fatal("expected error for non-numeric ID, got nil")
}

// Pattern 2: Check error message contains substring
if !strings.Contains(err.Error(), "invalid server ID") {
    t.Errorf("expected 'invalid server ID' in error, got: %v", err)
}

// Pattern 3: Check sentinel error with errors.Is
if !errors.Is(err, domain.ErrNotFound) {
    t.Errorf("expected ErrNotFound, got: %v", err)
}
```

**Struct Comparison:**
```go
if diff := cmp.Diff(want, got); diff != "" {
    t.Errorf("mismatch (-want +got):\n%s", diff)
}
```

**Transient Error / Retry Testing:**
```go
callCount := 0
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    callCount++
    if callCount == 1 {
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(errorResponse)
        return
    }
    json.NewEncoder(w).Encode(successResponse)
}))
t.Cleanup(srv.Close)
```

## Test File Inventory

| File | Lines | Tests |
|------|-------|-------|
| `internal/dns/providers/cloudflare_test.go` | 660 | Cloudflare DNS provider CRUD |
| `internal/server/providers/hetzner_test.go` | 639 | Hetzner server CRUD operations |
| `internal/server/providers/hetzner_catalog_test.go` | 618 | Hetzner catalog (locations, types, images) |
| `cmd/commands/server/start_test.go` | 488 | Server start CLI command |
| `internal/dns/providers/porkbun_test.go` | 429 | Porkbun DNS provider CRUD |
| `internal/dns/services/service_test.go` | 339 | DNS service business logic |
| `internal/server/tui/create_test.go` | 260 | Server create TUI |
| `internal/server/services/action/service_test.go` | ~200 | Action polling service |
| `cmd/commands/server/list_test.go` | 166 | Server list CLI command |
| `internal/server/providers/hetzner_metrics_test.go` | ~100 | Server metrics |
| `internal/cache/cache_test.go` | 78 | File-based cache |
| `internal/retry/retry_test.go` | 96 | Retry logic |
| `internal/config/config_test.go` | 105 | Config load/save |
| Others | ~400 | Various CLI commands, validation, etc. |

**Total test code:** ~7,600 lines across 30 test files.

---

*Testing analysis: 2026-03-20*
