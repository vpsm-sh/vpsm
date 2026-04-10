package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/google/go-cmp/cmp"
)

// --- Test helpers ---

// newTestCloudflareProvider creates a CloudflareProvider pointed at the given test server.
func newTestCloudflareProvider(t *testing.T, serverURL string) *CloudflareProvider {
	t.Helper()
	p := NewCloudflareProvider("test-token")
	p.baseURL = serverURL
	return p
}

// cfSuccessEnvelope returns a Cloudflare success envelope wrapping the given result.
func cfSuccessEnvelope(result any) map[string]any {
	return map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   result,
	}
}

// cfSuccessListEnvelope returns a Cloudflare success list envelope with pagination.
func cfSuccessListEnvelope(result []any, page, totalPages, totalCount int) map[string]any {
	return map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   result,
		"result_info": map[string]any{
			"page":        page,
			"per_page":    50,
			"total_pages": totalPages,
			"count":       len(result),
			"total_count": totalCount,
		},
	}
}

// cfErrorEnvelope returns a Cloudflare error envelope.
func cfErrorEnvelope(code int, message string) map[string]any {
	return map[string]any{
		"success":  false,
		"errors":   []any{map[string]any{"code": code, "message": message}},
		"messages": []any{},
		"result":   nil,
	}
}

// testCFZoneJSON returns a sample Cloudflare zone object.
func testCFZoneJSON(id, name, status string) map[string]any {
	return map[string]any{
		"id":         id,
		"name":       name,
		"status":     status,
		"created_on": "2024-01-01T00:00:00.000000Z",
	}
}

// testCFRecordJSON returns a sample Cloudflare DNS record object.
func testCFRecordJSON(id, name, typ, content string, ttl int, priority *int, comment string) map[string]any {
	rec := map[string]any{
		"id":        id,
		"zone_id":   "zone-123",
		"zone_name": "example.com",
		"name":      name,
		"type":      typ,
		"content":   content,
		"ttl":       ttl,
		"comment":   comment,
	}
	if priority != nil {
		rec["priority"] = *priority
	}
	return rec
}

// intPtr is a helper to create an *int.
//
//go:fix inline
func intPtr(v int) *int { return new(v) }

// newCFRouter creates a httptest.Server that routes requests based on method + path.
// The handler map keys are "METHOD /path" strings.
func newCFRouter(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip query string for matching.
		pathWithQuery := r.URL.Path
		key := r.Method + " " + pathWithQuery

		// Try exact match first, then with query.
		handler, ok := handlers[key]
		if !ok {
			// Try matching with the full URL (method + path + query).
			key = r.Method + " " + r.URL.String()
			handler, ok = handlers[key]
		}
		if !ok {
			// Try matching method + path exactly (ignoring query).
			for k, h := range handlers {
				parts := strings.SplitN(k, " ", 2)
				if len(parts) == 2 && parts[0] == r.Method && parts[1] == r.URL.Path {
					handler = h
					ok = true
					break
				}
			}
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(cfErrorEnvelope(0, fmt.Sprintf("no handler for %s %s", r.Method, r.URL.String())))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// --- ListDomains tests ---

func TestCloudflare_ListDomains_HappyPath(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			body := cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-1", "example.com", "active"),
				testCFZoneJSON("zone-2", "another.io", "active"),
			}, 1, 1, 2)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(body)
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := []domain.Domain{
		{Name: "example.com", Status: "active", TLD: "com", CreateDate: "2024-01-01T00:00:00.000000Z", ExpireDate: "N/A"},
		{Name: "another.io", Status: "active", TLD: "io", CreateDate: "2024-01-01T00:00:00.000000Z", ExpireDate: "N/A"},
	}

	if diff := cmp.Diff(want, domains); diff != "" {
		t.Errorf("ListDomains mismatch (-want +got):\n%s", diff)
	}
}

func TestCloudflare_ListDomains_Pagination(t *testing.T) {
	callCount := 0
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 1 {
				body := cfSuccessListEnvelope([]any{
					testCFZoneJSON("zone-1", "example.com", "active"),
				}, 1, 2, 2)
				json.NewEncoder(w).Encode(body)
			} else {
				body := cfSuccessListEnvelope([]any{
					testCFZoneJSON("zone-2", "another.io", "active"),
				}, 2, 2, 2)
				json.NewEncoder(w).Encode(body)
			}
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestCloudflare_ListDomains_EmptyList(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			body := cfSuccessListEnvelope([]any{}, 1, 1, 0)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(body)
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(domains))
	}
}

func TestCloudflare_ListDomains_Unauthorized(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(cfErrorEnvelope(9109, "Invalid access token"))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	_, err := p.ListDomains(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

// --- ListRecords tests ---

func TestCloudflare_ListRecords_HappyPath(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"GET /zones/zone-123/dns_records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFRecordJSON("rec-1", "example.com", "A", "1.2.3.4", 300, nil, ""),
				testCFRecordJSON("rec-2", "www.example.com", "A", "1.2.3.4", 300, nil, ""),
				testCFRecordJSON("rec-3", "example.com", "MX", "mail.example.com", 3600, new(10), "mail server"),
			}, 1, 1, 3))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	records, err := p.ListRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	if records[0].ID != "rec-1" {
		t.Errorf("records[0].ID = %q, want %q", records[0].ID, "rec-1")
	}
	if records[0].TTL != 300 {
		t.Errorf("records[0].TTL = %d, want 300", records[0].TTL)
	}
	if records[2].Priority != 10 {
		t.Errorf("records[2].Priority = %d, want 10", records[2].Priority)
	}
	if records[2].Notes != "mail server" {
		t.Errorf("records[2].Notes = %q, want %q", records[2].Notes, "mail server")
	}
}

func TestCloudflare_ListRecords_ZoneNotFound(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{}, 1, 1, 0))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	_, err := p.ListRecords(context.Background(), "notexist.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- GetRecord tests ---

func TestCloudflare_GetRecord_HappyPath(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"GET /zones/zone-123/dns_records/rec-1": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessEnvelope(
				testCFRecordJSON("rec-1", "example.com", "A", "1.2.3.4", 300, nil, "test note"),
			))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	rec, err := p.GetRecord(context.Background(), "example.com", "rec-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := &domain.Record{
		ID:       "rec-1",
		Domain:   "example.com",
		Name:     "example.com",
		Type:     domain.RecordTypeA,
		Content:  "1.2.3.4",
		TTL:      300,
		Priority: 0,
		Notes:    "test note",
	}

	if diff := cmp.Diff(want, rec); diff != "" {
		t.Errorf("GetRecord mismatch (-want +got):\n%s", diff)
	}
}

func TestCloudflare_GetRecord_NotFound(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"GET /zones/zone-123/dns_records/rec-999": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(cfErrorEnvelope(81044, "Record not found"))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	_, err := p.GetRecord(context.Background(), "example.com", "rec-999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- CreateRecord tests ---

func TestCloudflare_CreateRecord_HappyPath(t *testing.T) {
	var capturedBody cfCreateRecordBody
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"POST /zones/zone-123/dns_records": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessEnvelope(
				testCFRecordJSON("rec-new", "www.example.com", "A", "5.6.7.8", 300, nil, ""),
			))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	rec, err := p.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Name:    "www",
		Type:    domain.RecordTypeA,
		Content: "5.6.7.8",
		TTL:     300,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rec.ID != "rec-new" {
		t.Errorf("rec.ID = %q, want %q", rec.ID, "rec-new")
	}
	if rec.Content != "5.6.7.8" {
		t.Errorf("rec.Content = %q, want %q", rec.Content, "5.6.7.8")
	}

	// Verify the request body had the FQDN.
	if capturedBody.Name != "www.example.com" {
		t.Errorf("request body name = %q, want %q", capturedBody.Name, "www.example.com")
	}
}

func TestCloudflare_CreateRecord_RootDomain(t *testing.T) {
	var capturedBody cfCreateRecordBody
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"POST /zones/zone-123/dns_records": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessEnvelope(
				testCFRecordJSON("rec-root", "example.com", "A", "1.2.3.4", 300, nil, ""),
			))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	rec, err := p.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Name:    "",
		Type:    domain.RecordTypeA,
		Content: "1.2.3.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rec.ID != "rec-root" {
		t.Errorf("rec.ID = %q, want %q", rec.ID, "rec-root")
	}

	// Root domain should use just the domain name, not ".example.com".
	if capturedBody.Name != "example.com" {
		t.Errorf("request body name = %q, want %q", capturedBody.Name, "example.com")
	}
}

func TestCloudflare_CreateRecord_Conflict(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"POST /zones/zone-123/dns_records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(cfErrorEnvelope(81057, "Record already exists"))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	_, err := p.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "1.1.1.1",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
}

// --- UpdateRecord tests ---

func TestCloudflare_UpdateRecord_HappyPath(t *testing.T) {
	var capturedMethod string
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"PATCH /zones/zone-123/dns_records/rec-1": func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessEnvelope(
				testCFRecordJSON("rec-1", "example.com", "A", "9.9.9.9", 1800, nil, ""),
			))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	err := p.UpdateRecord(context.Background(), "example.com", "rec-1", domain.UpdateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "9.9.9.9",
		TTL:     1800,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedMethod != "PATCH" {
		t.Errorf("expected PATCH method, got %s", capturedMethod)
	}
}

func TestCloudflare_UpdateRecord_NotFound(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"PATCH /zones/zone-123/dns_records/rec-999": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(cfErrorEnvelope(81044, "Record not found"))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	err := p.UpdateRecord(context.Background(), "example.com", "rec-999", domain.UpdateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "1.1.1.1",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- DeleteRecord tests ---

func TestCloudflare_DeleteRecord_HappyPath(t *testing.T) {
	var capturedPath string
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"DELETE /zones/zone-123/dns_records/rec-1": func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessEnvelope(
				map[string]any{"id": "rec-1"},
			))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	err := p.DeleteRecord(context.Background(), "example.com", "rec-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedPath != "/zones/zone-123/dns_records/rec-1" {
		t.Errorf("expected path /zones/zone-123/dns_records/rec-1, got %s", capturedPath)
	}
}

func TestCloudflare_DeleteRecord_NotFound(t *testing.T) {
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{
				testCFZoneJSON("zone-123", "example.com", "active"),
			}, 1, 1, 1))
		},
		"DELETE /zones/zone-123/dns_records/rec-999": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(cfErrorEnvelope(81044, "Record not found"))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	err := p.DeleteRecord(context.Background(), "example.com", "rec-999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- Auth header tests ---

func TestCloudflare_BearerTokenSent(t *testing.T) {
	var capturedAuth string
	srv := newCFRouter(t, map[string]http.HandlerFunc{
		"GET /zones": func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfSuccessListEnvelope([]any{}, 1, 1, 0))
		},
	})

	p := newTestCloudflareProvider(t, srv.URL)

	p.ListDomains(context.Background())

	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Authorization = %q, got %q", "Bearer test-token", capturedAuth)
	}
}

// --- Registry tests ---

func TestCloudflare_Registry_RegisterAndGet(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	store.SetToken(cloudflareTokenStore, "cf-test-token")

	RegisterCloudflare()

	p, err := Get("cloudflare", store)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.GetDisplayName() != "Cloudflare" {
		t.Errorf("GetDisplayName = %q, want %q", p.GetDisplayName(), "Cloudflare")
	}
}

func TestCloudflare_Registry_MissingToken(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	// No token set.

	RegisterCloudflare()

	_, err := Get("cloudflare", store)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}
