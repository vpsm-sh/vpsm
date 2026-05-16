package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nathanbeddoewebdev/vpsm/internal/server/domain"

	"github.com/google/go-cmp/cmp"
)

func TestVultrCatalogProvider_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response any
		switch r.URL.Path {
		case "/v2/regions":
			response = map[string]any{
				"regions": []any{
					map[string]any{
						"id":      "ewr",
						"city":    "New Jersey",
						"country": "US",
						"options": []string{"ddos_protection"},
					},
				},
				"meta": map[string]any{"links": map[string]any{}},
			}
		case "/v2/plans":
			response = map[string]any{
				"plans": []any{
					map[string]any{
						"id":           "vc2-1c-2gb",
						"vcpu_count":   1,
						"ram":          2048,
						"disk":         55,
						"monthly_cost": 12.0,
						"type":         "vc2",
						"locations":    []string{"ewr", "lhr"},
					},
				},
				"meta": map[string]any{"links": map[string]any{}},
			}
		case "/v2/os":
			response = map[string]any{
				"os": []any{
					map[string]any{
						"id":     2284,
						"name":   "Ubuntu 24.04 LTS x64",
						"arch":   "x64",
						"family": "ubuntu",
					},
				},
				"meta": map[string]any{"links": map[string]any{}},
			}
		case "/v2/ssh-keys":
			response = map[string]any{
				"ssh_keys": []any{
					map[string]any{
						"id":      "ssh-key-1",
						"name":    "workstation",
						"ssh_key": "ssh-ed25519 AAAA test",
					},
				},
				"meta": map[string]any{"links": map[string]any{}},
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	provider := newTestVultrProvider(t, srv.URL, "test-token")

	locations, err := provider.ListLocations(context.Background())
	if err != nil {
		t.Fatalf("ListLocations returned error: %v", err)
	}
	wantLocations := []domain.Location{
		{ID: "ewr", Name: "ewr", Description: "New Jersey", Country: "US", City: "New Jersey"},
	}
	if diff := cmp.Diff(wantLocations, locations); diff != "" {
		t.Fatalf("locations mismatch (-want +got):\n%s", diff)
	}

	serverTypes, err := provider.ListServerTypes(context.Background())
	if err != nil {
		t.Fatalf("ListServerTypes returned error: %v", err)
	}
	wantServerTypes := []domain.ServerTypeSpec{
		{
			ID:           "vc2-1c-2gb",
			Name:         "vc2-1c-2gb",
			Description:  "vc2-1c-2gb",
			Cores:        1,
			Memory:       2,
			Disk:         55,
			PriceMonthly: "12.00",
			Locations:    []string{"ewr", "lhr"},
		},
	}
	if diff := cmp.Diff(wantServerTypes, serverTypes); diff != "" {
		t.Fatalf("server types mismatch (-want +got):\n%s", diff)
	}

	images, err := provider.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages returned error: %v", err)
	}
	wantImages := []domain.ImageSpec{
		{
			ID:           "2284",
			Name:         "2284",
			Description:  "Ubuntu 24.04 LTS x64",
			Type:         "system",
			OSFlavor:     "ubuntu",
			Architecture: "x64",
		},
	}
	if diff := cmp.Diff(wantImages, images); diff != "" {
		t.Fatalf("images mismatch (-want +got):\n%s", diff)
	}

	keys, err := provider.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys returned error: %v", err)
	}
	wantKeys := []domain.SSHKeySpec{
		{ID: "ssh-key-1", Name: "workstation"},
	}
	if diff := cmp.Diff(wantKeys, keys); diff != "" {
		t.Fatalf("SSH keys mismatch (-want +got):\n%s", diff)
	}
}

func TestVultrListServerTypes_Paginates(t *testing.T) {
	var cursors []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Query().Get("cursor") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"plans": []any{
					map[string]any{"id": "vc2-1c-2gb", "ram": 2048},
				},
				"meta": map[string]any{
					"links": map[string]any{"next": "next-cursor"},
				},
			})
		case "next-cursor":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"plans": []any{
					map[string]any{"id": "vc2-2c-4gb", "ram": 4096},
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
	serverTypes, err := provider.ListServerTypes(context.Background())
	if err != nil {
		t.Fatalf("ListServerTypes returned error: %v", err)
	}
	if len(serverTypes) != 2 {
		t.Fatalf("len(serverTypes) = %d, want 2", len(serverTypes))
	}

	wantCursors := []string{"", "next-cursor"}
	if diff := cmp.Diff(wantCursors, cursors); diff != "" {
		t.Fatalf("cursors mismatch (-want +got):\n%s", diff)
	}
}
