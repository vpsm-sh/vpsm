package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nathanbeddoewebdev/vpsm/internal/cache"
	"nathanbeddoewebdev/vpsm/internal/retry"
	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/google/go-cmp/cmp"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
)

// --- Test helpers ---

// newTestHetznerProvider creates a HetznerProvider whose SDK client
// is pointed at the given httptest server URL.
func newTestHetznerProvider(t *testing.T, serverURL string, token string) *HetznerProvider {
	t.Helper()
	provider := NewHetznerProvider(
		hcloud.WithEndpoint(serverURL),
		hcloud.WithToken(token),
	)
	provider.cache = cache.New(t.TempDir())
	provider.retryConfig = retry.Config{MaxAttempts: 3}
	return provider
}

// newTestAPI spins up an httptest.Server that returns the given response as JSON.
// The server is automatically closed when the test finishes.
func newTestAPI(t *testing.T, response any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode test response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- JSON builder helpers for Hetzner API-shaped responses ---

// testLocationJSON builds a Hetzner API location object.
func testLocationJSON(id int, name, country, city string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "description": name,
		"country": country, "city": city,
		"latitude": 50.0, "longitude": 12.0,
		"network_zone": "eu-central",
	}
}

// testServerTypeJSON builds a Hetzner API server_type object.
func testServerTypeJSON(id int, name, arch string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "description": name,
		"cores": 2, "memory": 2.0, "disk": 40,
		"architecture": arch,
		"storage_type": "local",
		"cpu_type":     "shared",
		"prices":       []any{},
		"locations":    []any{},
	}
}

// testServerTypeLocationJSON builds a Hetzner API server_type location entry,
// optionally with deprecation info.
func testServerTypeLocationJSON(id int, name string, deprecation map[string]any) map[string]any {
	loc := map[string]any{
		"id":   id,
		"name": name,
	}
	if deprecation != nil {
		loc["deprecation"] = deprecation
	}
	return loc
}

// testImageJSON builds a Hetzner API image object.
func testImageJSON(id int, name, osFlavor, osVersion, arch string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "description": name,
		"type": "system", "status": "available",
		"os_flavor": osFlavor, "os_version": osVersion,
		"architecture": arch,
	}
}

// testServerJSON builds a minimal Hetzner API server object with sensible defaults.
// The returned map can be modified before being used in a response.
func testServerJSON(id int, name, status, created string, loc map[string]any, st map[string]any) map[string]any {
	return map[string]any{
		"id":      id,
		"name":    name,
		"status":  status,
		"created": created,
		"public_net": map[string]any{
			"floating_ips": []any{},
			"firewalls":    []any{},
		},
		"private_net":    []any{},
		"server_type":    st,
		"image":          nil,
		"location":       loc,
		"labels":         map[string]any{},
		"volumes":        []any{},
		"load_balancers": []any{},
	}
}

// --- ListServers tests ---

func TestListServers_HappyPath(t *testing.T) {
	const createdStr = "2024-06-15T12:00:00+00:00"
	// Parse the expected time identically to how the SDK will parse it from JSON,
	// avoiding any timezone representation mismatches with time.Date().
	created, _ := time.Parse(time.RFC3339, createdStr)

	fsn1 := testLocationJSON(1, "fsn1", "DE", "Falkenstein")
	nbg1 := testLocationJSON(2, "nbg1", "DE", "Nuremberg")

	server1 := testServerJSON(42, "web-server", "running", createdStr, fsn1, testServerTypeJSON(1, "cpx11", "x86"))
	server1["public_net"] = map[string]any{
		"ipv4":         map[string]any{"ip": "1.2.3.4", "blocked": false},
		"ipv6":         map[string]any{"ip": "2001:db8::/64", "blocked": false},
		"floating_ips": []any{},
		"firewalls":    []any{},
	}
	server1["private_net"] = []any{
		map[string]any{"ip": "10.0.0.2", "alias_ips": []any{}, "network": 1, "mac_address": ""},
	}
	server1["image"] = testImageJSON(1, "ubuntu-24.04", "ubuntu", "24.04", "x86")

	server2 := testServerJSON(99, "db-server", "stopped", createdStr, nbg1, testServerTypeJSON(2, "cpx22", "arm"))
	server2["public_net"] = map[string]any{
		"ipv4":         map[string]any{"ip": "5.6.7.8", "blocked": false},
		"floating_ips": []any{},
		"firewalls":    []any{},
	}
	server2["image"] = testImageJSON(2, "debian-12", "debian", "12", "x86")

	response := map[string]any{
		"servers": []any{server1, server2},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/servers" {
			t.Errorf("expected path /servers, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header 'Bearer test-token', got %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	servers, err := provider.ListServers(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	wantFirst := domain.Server{
		ID:          "42",
		Name:        "web-server",
		Status:      "running",
		CreatedAt:   created,
		PublicIPv4:  "1.2.3.4",
		PublicIPv6:  "2001:db8::",
		PrivateIPv4: "10.0.0.2",
		Region:      "fsn1",
		ServerType:  "cpx11",
		Image:       "ubuntu-24.04",
		Provider:    "hetzner",
		Metadata: map[string]any{
			"hetzner_id":   int64(42),
			"architecture": "x86",
		},
	}

	wantSecond := domain.Server{
		ID:         "99",
		Name:       "db-server",
		Status:     "stopped",
		CreatedAt:  created,
		PublicIPv4: "5.6.7.8",
		Region:     "nbg1",
		ServerType: "cpx22",
		Image:      "debian-12",
		Provider:   "hetzner",
		Metadata: map[string]any{
			"hetzner_id":   int64(99),
			"architecture": "arm",
		},
	}

	if diff := cmp.Diff(wantFirst, servers[0]); diff != "" {
		t.Errorf("server[0] mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantSecond, servers[1]); diff != "" {
		t.Errorf("server[1] mismatch (-want +got):\n%s", diff)
	}
}

func TestListServers_EmptyList(t *testing.T) {
	srv := newTestAPI(t, map[string]any{
		"servers": []any{},
	})

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	servers, err := provider.ListServers(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestListServers_RetriesOnTransientError(t *testing.T) {
	createdStr := "2024-06-15T12:00:00+00:00"
	loc := testLocationJSON(1, "fsn1", "DE", "Falkenstein")
	st := testServerTypeJSON(1, "cpx11", "x86")
	server := testServerJSON(42, "retry-server", "running", createdStr, loc, st)

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "service_error",
					"message": "temporary error",
				},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"servers": []any{server},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	servers, err := provider.ListServers(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if callCount != 2 {
		t.Fatalf("expected 2 API calls, got %d", callCount)
	}
}

func TestListServers_NilOptionalFields(t *testing.T) {
	loc := testLocationJSON(3, "hel1", "FI", "Helsinki")

	server := testServerJSON(1, "bare-server", "running", "2024-06-15T12:00:00+00:00", loc, testServerTypeJSON(1, "cx11", "x86"))
	// image is already nil from testServerJSON
	// public_net has no ipv4/ipv6 entries, private_net is empty

	srv := newTestAPI(t, map[string]any{
		"servers": []any{server},
	})

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	servers, err := provider.ListServers(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	s := servers[0]
	if s.PublicIPv4 != "" {
		t.Errorf("PublicIPv4 = %q, want empty", s.PublicIPv4)
	}
	if s.PublicIPv6 != "" {
		t.Errorf("PublicIPv6 = %q, want empty", s.PublicIPv6)
	}
	if s.PrivateIPv4 != "" {
		t.Errorf("PrivateIPv4 = %q, want empty", s.PrivateIPv4)
	}
	if s.Image != "" {
		t.Errorf("Image = %q, want empty", s.Image)
	}
}

func TestListServers_Non200StatusCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "unauthorized",
				"message": "unable to authenticate",
			},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "bad-token")
	_, err := provider.ListServers(ctx)
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestListServers_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{invalid json`))
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	_, err := provider.ListServers(ctx)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestListServers_FactoryViaRegistry(t *testing.T) {
	loc := testLocationJSON(4, "ash", "US", "Ashburn")

	server := testServerJSON(7, "registry-server", "running", "2024-06-15T12:00:00+00:00", loc, testServerTypeJSON(3, "cpx31", "x86"))

	response := map[string]any{
		"servers": []any{server},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer registry-token" {
			t.Errorf("expected Authorization 'Bearer registry-token', got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(srv.Close)

	Reset()
	t.Cleanup(func() { Reset() })

	Register("test-hetzner", func(store auth.Store) (domain.Provider, error) {
		token, err := store.GetToken("test-hetzner")
		if err != nil {
			return nil, err
		}
		return NewHetznerProvider(
			hcloud.WithToken(token),
			hcloud.WithEndpoint(srv.URL),
		), nil
	})

	store := auth.NewMockStore()
	store.SetToken("test-hetzner", "registry-token")

	provider, err := Get("test-hetzner", store)
	if err != nil {
		t.Fatalf("expected no error from Get, got %v", err)
	}

	ctx := context.Background()
	servers, err := provider.ListServers(ctx)
	if err != nil {
		t.Fatalf("expected no error from ListServers, got %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Name != "registry-server" {
		t.Errorf("server.Name = %q, want %q", servers[0].Name, "registry-server")
	}
	if servers[0].Provider != "hetzner" {
		t.Errorf("server.Provider = %q, want %q", servers[0].Provider, "hetzner")
	}
}

func TestListServers_FactoryMissingToken(t *testing.T) {
	Reset()
	t.Cleanup(func() { Reset() })

	Register("test-hetzner", func(store auth.Store) (domain.Provider, error) {
		token, err := store.GetToken("test-hetzner")
		if err != nil {
			return nil, err
		}
		return NewHetznerProvider(hcloud.WithToken(token)), nil
	})

	store := auth.NewMockStore() // no token set

	_, err := Get("test-hetzner", store)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}

// --- GetServer tests ---

func TestGetServer_HappyPath(t *testing.T) {
	const createdStr = "2024-06-15T12:00:00+00:00"
	created, _ := time.Parse(time.RFC3339, createdStr)

	fsn1 := testLocationJSON(1, "fsn1", "DE", "Falkenstein")

	server := testServerJSON(42, "web-server", "running", createdStr, fsn1, testServerTypeJSON(1, "cpx11", "x86"))
	server["public_net"] = map[string]any{
		"ipv4":         map[string]any{"ip": "1.2.3.4", "blocked": false},
		"ipv6":         map[string]any{"ip": "2001:db8::/64", "blocked": false},
		"floating_ips": []any{},
		"firewalls":    []any{},
	}
	server["image"] = testImageJSON(1, "ubuntu-24.04", "ubuntu", "24.04", "x86")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		if r.URL.Path != "/servers/42" {
			t.Errorf("expected path /servers/42, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"server": server,
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	got, err := provider.GetServer(ctx, "42")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := &domain.Server{
		ID:         "42",
		Name:       "web-server",
		Status:     "running",
		CreatedAt:  created,
		PublicIPv4: "1.2.3.4",
		PublicIPv6: "2001:db8::",
		Region:     "fsn1",
		ServerType: "cpx11",
		Image:      "ubuntu-24.04",
		Provider:   "hetzner",
		Metadata: map[string]any{
			"hetzner_id":   int64(42),
			"architecture": "x86",
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetServer mismatch (-want +got):\n%s", diff)
	}
}

func TestGetServer_InvalidID(t *testing.T) {
	ctx := context.Background()
	provider := newTestHetznerProvider(t, "http://unused", "test-token")
	_, err := provider.GetServer(ctx, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric ID, got nil")
	}
	if !strings.Contains(err.Error(), "invalid server ID") {
		t.Errorf("expected 'invalid server ID' in error, got: %v", err)
	}
}

func TestGetServer_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "not_found",
				"message": "server with ID '999' not found",
			},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	_, err := provider.GetServer(ctx, "999")
	if err == nil {
		t.Fatal("expected error for non-existent server, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestGetServer_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "unauthorized",
				"message": "unable to authenticate",
			},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "bad-token")
	_, err := provider.GetServer(ctx, "42")
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

// --- DeleteServer tests ---

func TestDeleteServer_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE method, got %s", r.Method)
		}
		if r.URL.Path != "/servers/42" {
			t.Errorf("expected path /servers/42, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization 'Bearer test-token', got %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"action": map[string]any{
				"id":       1,
				"status":   "running",
				"command":  "delete_server",
				"progress": 0,
				"resources": []any{
					map[string]any{"id": 42, "type": "server"},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	err := provider.DeleteServer(ctx, "42")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDeleteServer_InvalidID(t *testing.T) {
	ctx := context.Background()
	provider := newTestHetznerProvider(t, "http://unused", "test-token")
	err := provider.DeleteServer(ctx, "not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric ID, got nil")
	}
	if !strings.Contains(err.Error(), "invalid server ID") {
		t.Errorf("expected 'invalid server ID' in error, got: %v", err)
	}
}

func TestDeleteServer_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "not_found",
				"message": "server with ID '999' not found",
			},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	err := provider.DeleteServer(ctx, "999")
	if err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}

func TestDeleteServer_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "server_error",
				"message": "internal server error",
			},
		})
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	provider := newTestHetznerProvider(t, srv.URL, "test-token")
	err := provider.DeleteServer(ctx, "42")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "failed to delete server") {
		t.Errorf("expected 'failed to delete server' in error, got: %v", err)
	}
}
