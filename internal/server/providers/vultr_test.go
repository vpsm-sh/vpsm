package providers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/google/go-cmp/cmp"
	"github.com/vultr/govultr/v3"
)

func newTestVultrProvider(t *testing.T, serverURL, token string) *VultrProvider {
	t.Helper()
	provider := NewVultrProvider(token)
	baseURL, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("failed to parse test URL: %v", err)
	}
	provider.client.BaseURL = baseURL
	provider.client.SetRetryLimit(0)
	provider.client.SetRateLimit(0)
	return provider
}

func TestVultrNormalizeStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		power  string
		want   string
	}{
		{
			name:   "pending running initializes",
			status: "pending",
			power:  "running",
			want:   "initializing",
		},
		{
			name:   "active running",
			status: "active",
			power:  "running",
			want:   "running",
		},
		{
			name:   "active stopped",
			status: "active",
			power:  "stopped",
			want:   "off",
		},
		{
			name:   "unknown status takes precedence",
			status: "maintenance",
			power:  "running",
			want:   "maintenance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &govultr.Instance{
				Status:      tt.status,
				PowerStatus: tt.power,
			}

			got := normalizeVultrStatus(instance)
			if got != tt.want {
				t.Fatalf("normalizeVultrStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVultrListServers_HappyPath(t *testing.T) {
	const createdStr = "2026-05-01T10:30:00+00:00"
	created, err := time.Parse(time.RFC3339, createdStr)
	if err != nil {
		t.Fatalf("failed to parse expected time: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/instances" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/v2/instances")
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q, want bearer token", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"instances": []any{
				map[string]any{
					"id":                "instance-1",
					"label":             "web-1",
					"hostname":          "web-1.local",
					"os":                "Ubuntu 24.04 LTS x64",
					"os_id":             2284,
					"ram":               2048,
					"disk":              55,
					"plan":              "vc2-1c-2gb",
					"main_ip":           "203.0.113.10",
					"v6_main_ip":        "2001:db8::10",
					"internal_ip":       "10.1.0.5",
					"vcpu_count":        1,
					"region":            "ewr",
					"date_created":      createdStr,
					"power_status":      "running",
					"server_status":     "ok",
					"status":            "active",
					"allowed_bandwidth": 2,
					"features":          []string{"ipv6"},
					"tags":              []string{"env:test"},
				},
			},
			"meta": map[string]any{"links": map[string]any{}},
		}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")
	servers, err := provider.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers returned error: %v", err)
	}

	want := []domain.Server{
		{
			ID:          "instance-1",
			Name:        "web-1",
			Status:      "running",
			CreatedAt:   created,
			PublicIPv4:  "203.0.113.10",
			PublicIPv6:  "2001:db8::10",
			PrivateIPv4: "10.1.0.5",
			Region:      "ewr",
			ServerType:  "vc2-1c-2gb",
			Image:       "Ubuntu 24.04 LTS x64",
			Provider:    "vultr",
			Metadata: map[string]any{
				"hostname":          "web-1.local",
				"os_id":             2284,
				"ram_mb":            2048,
				"disk_gb":           55,
				"vcpu_count":        1,
				"allowed_bandwidth": 2,
				"server_status":     "ok",
				"features":          []string{"ipv6"},
				"tags":              []string{"env:test"},
			},
		},
	}

	if diff := cmp.Diff(want, servers); diff != "" {
		t.Fatalf("servers mismatch (-want +got):\n%s", diff)
	}
}

func TestVultrProvider_StripsBearerPrefixFromToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"instances": []any{},
			"meta":      map[string]any{"links": map[string]any{}},
		})
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "Bearer test-token")
	if _, err := provider.ListServers(context.Background()); err != nil {
		t.Fatalf("ListServers returned error: %v", err)
	}
}

func TestVultrCreateServer_HappyPath(t *testing.T) {
	const createdStr = "2026-05-01T10:30:00+00:00"
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodGet && r.URL.Path == "/v2/ssh-keys" {
			if err := json.NewEncoder(w).Encode(map[string]any{
				"ssh_keys": []any{
					map[string]any{"id": "ssh-key-uuid-1", "name": "Mac Pro"},
				},
				"meta": map[string]any{"links": map[string]any{}},
			}); err != nil {
				t.Fatalf("failed to encode SSH key response: %v", err)
			}
			return
		}

		if r.URL.Path != "/v2/instances" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/v2/instances")
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if err := json.NewEncoder(w).Encode(map[string]any{
			"instance": map[string]any{
				"id":           "instance-2",
				"label":        "api-1",
				"hostname":     "api-1",
				"os":           "Ubuntu 24.04 LTS x64",
				"os_id":        2284,
				"plan":         "vc2-1c-2gb",
				"region":       "ewr",
				"date_created": createdStr,
				"power_status": "running",
				"status":       "active",
			},
		}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")
	server, err := provider.CreateServer(context.Background(), domain.CreateServerOpts{
		Name:              "api-1",
		Image:             "2284",
		ServerType:        "vc2-1c-2gb",
		Location:          "ewr",
		SSHKeyIdentifiers: []string{"Mac Pro"},
		Labels:            map[string]string{"env": "test"},
		UserData:          "#cloud-config",
	})
	if err != nil {
		t.Fatalf("CreateServer returned error: %v", err)
	}

	wantBody := map[string]any{
		"region":    "ewr",
		"plan":      "vc2-1c-2gb",
		"label":     "api-1",
		"hostname":  "api-1",
		"os_id":     float64(2284),
		"sshkey_id": []any{"ssh-key-uuid-1"},
		"tags":      []any{"env:test"},
		"user_data": "#cloud-config",
	}
	for key, want := range wantBody {
		if diff := cmp.Diff(want, gotBody[key]); diff != "" {
			t.Fatalf("request field %s mismatch (-want +got):\n%s", key, diff)
		}
	}

	if server.ID != "instance-2" {
		t.Fatalf("server.ID = %q, want %q", server.ID, "instance-2")
	}
	if server.Provider != "vultr" {
		t.Fatalf("server.Provider = %q, want %q", server.Provider, "vultr")
	}
}

func TestVultrServerLifecycleMethods(t *testing.T) {
	var calls []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/instances/instance-3":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"instance": map[string]any{
					"id":           "instance-3",
					"label":        "db-1",
					"plan":         "vc2-1c-2gb",
					"region":       "ewr",
					"power_status": "stopped",
				},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v2/instances/instance-3":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/instances/instance-3/start":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/instances/instance-3/halt":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")
	server, err := provider.GetServer(context.Background(), "instance-3")
	if err != nil {
		t.Fatalf("GetServer returned error: %v", err)
	}
	if server.Status != "off" {
		t.Fatalf("server.Status = %q, want %q", server.Status, "off")
	}

	startAction, err := provider.StartServer(context.Background(), "instance-3")
	if err != nil {
		t.Fatalf("StartServer returned error: %v", err)
	}
	if startAction.Status != domain.ActionStatusRunning || startAction.Command != "start_server" {
		t.Fatalf("unexpected start action: %+v", startAction)
	}

	stopAction, err := provider.StopServer(context.Background(), "instance-3")
	if err != nil {
		t.Fatalf("StopServer returned error: %v", err)
	}
	if stopAction.Status != domain.ActionStatusRunning || stopAction.Command != "stop_server" {
		t.Fatalf("unexpected stop action: %+v", stopAction)
	}

	if err := provider.DeleteServer(context.Background(), "instance-3"); err != nil {
		t.Fatalf("DeleteServer returned error: %v", err)
	}

	gotCalls := strings.Join(calls, "\n")
	wantCalls := strings.Join([]string{
		"GET /v2/instances/instance-3",
		"GET /v2/instances/instance-3",
		"POST /v2/instances/instance-3/start",
		"POST /v2/instances/instance-3/halt",
		"DELETE /v2/instances/instance-3",
	}, "\n")
	if gotCalls != wantCalls {
		t.Fatalf("calls:\n%s\nwant:\n%s", gotCalls, wantCalls)
	}
}

func TestVultrStartServer_AlreadyRunningNoop(t *testing.T) {
	var calls []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/instances/instance-4":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"instance": map[string]any{
					"id":           "instance-4",
					"label":        "web-1",
					"plan":         "vc2-1c-2gb",
					"region":       "ewr",
					"power_status": "running",
					"status":       "active",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v2/instances/instance-4/start":
			t.Errorf("unexpected start request for already-running instance")
			w.WriteHeader(http.StatusConflict)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")
	action, err := provider.StartServer(context.Background(), "instance-4")
	if err != nil {
		t.Fatalf("StartServer returned error: %v", err)
	}

	wantAction := &domain.ActionStatus{
		Status:   domain.ActionStatusSuccess,
		Progress: 100,
		Command:  "start_server",
	}
	if diff := cmp.Diff(wantAction, action); diff != "" {
		t.Fatalf("action mismatch (-want +got):\n%s", diff)
	}

	wantCalls := "GET /v2/instances/instance-4"
	if gotCalls := strings.Join(calls, "\n"); gotCalls != wantCalls {
		t.Fatalf("calls:\n%s\nwant:\n%s", gotCalls, wantCalls)
	}
}

func TestRegisterVultr(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	if err := store.SetToken("vultr", "test-token"); err != nil {
		t.Fatalf("SetToken returned error: %v", err)
	}

	RegisterVultr()
	provider, err := Get("vultr", store)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if provider.GetDisplayName() != "Vultr" {
		t.Fatalf("GetDisplayName = %q, want %q", provider.GetDisplayName(), "Vultr")
	}
}

func TestVultrListServers_Paginates(t *testing.T) {
	var cursors []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Query().Get("cursor") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"instances": []any{
					map[string]any{"id": "instance-1", "label": "one"},
				},
				"meta": map[string]any{
					"links": map[string]any{"next": "next-cursor"},
				},
			})
		case "next-cursor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"instances": []any{
					map[string]any{"id": "instance-2", "label": "two"},
				},
				"meta": map[string]any{"links": map[string]any{}},
			})
		default:
			t.Errorf("unexpected cursor %q", r.URL.Query().Get("cursor"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")
	servers, err := provider.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers returned error: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("len(servers) = %d, want 2", len(servers))
	}

	wantCursors := []string{"", "next-cursor"}
	if diff := cmp.Diff(wantCursors, cursors); diff != "" {
		t.Fatalf("cursors mismatch (-want +got):\n%s", diff)
	}
}

func TestVultrListServers_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")
	_, err := provider.ListServers(context.Background())
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("ListServers error = %v, want ErrUnauthorized", err)
	}
}
