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

// newTestVercelProvider creates a VercelProvider pointed at the given test server.
func newTestVercelProvider(t *testing.T, serverURL string) *VercelProvider {
	t.Helper()
	p := NewVercelProvider("test-token", "")
	p.baseURL = serverURL
	return p
}

// newTestVercelProviderWithTeam creates a VercelProvider with a team ID.
func newTestVercelProviderWithTeam(t *testing.T, serverURL, teamID string) *VercelProvider {
	t.Helper()
	p := NewVercelProvider("test-token", teamID)
	p.baseURL = serverURL
	return p
}

// newVercelRouter creates a httptest.Server that routes requests based on method + path.
func newVercelRouter(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path

		handler, ok := handlers[key]
		if !ok {
			// Try matching method + path prefix.
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
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"code":    "not_found",
					"message": fmt.Sprintf("no handler for %s %s", r.Method, r.URL.String()),
				},
			})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// testVercelDomainJSON returns a sample Vercel domain object.
func testVercelDomainJSON(name string, verified bool, expiresAt *int64) map[string]any {
	d := map[string]any{
		"name":        name,
		"verified":    verified,
		"serviceType": "zeit.world",
		"createdAt":   int64(1613602938882),
		"renew":       true,
	}
	if expiresAt != nil {
		d["expiresAt"] = *expiresAt
	}
	return d
}

// testVercelRecordJSON returns a sample Vercel DNS record object.
func testVercelRecordJSON(id, name, typ, value string, ttl int, mxPriority *int, comment string) map[string]any {
	rec := map[string]any{
		"id":      id,
		"name":    name,
		"type":    typ,
		"value":   value,
		"ttl":     ttl,
		"comment": comment,
		"creator": "user123",
	}
	if mxPriority != nil {
		rec["mxPriority"] = *mxPriority
	}
	return rec
}

// --- ListDomains tests ---

func TestVercel_ListDomains_HappyPath(t *testing.T) {
	expires := int64(1700000000000)
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"domains": []any{
					testVercelDomainJSON("example.com", true, &expires),
					testVercelDomainJSON("another.io", false, nil),
				},
				"pagination": map[string]any{"count": 2, "next": 0, "prev": 0},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := []domain.Domain{
		{Name: "example.com", Status: "active", TLD: "com", CreateDate: "2021-02-17", ExpireDate: "2023-11-14"},
		{Name: "another.io", Status: "inactive", TLD: "io", CreateDate: "2021-02-17", ExpireDate: "N/A"},
	}

	if diff := cmp.Diff(want, domains); diff != "" {
		t.Errorf("ListDomains mismatch (-want +got):\n%s", diff)
	}
}

func TestVercel_ListDomains_Pagination(t *testing.T) {
	callCount := 0
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			if callCount == 1 {
				json.NewEncoder(w).Encode(map[string]any{
					"domains":    []any{testVercelDomainJSON("first.com", true, nil)},
					"pagination": map[string]any{"count": 1, "next": 1613602938882, "prev": 0},
				})
			} else {
				json.NewEncoder(w).Encode(map[string]any{
					"domains":    []any{testVercelDomainJSON("second.com", true, nil)},
					"pagination": map[string]any{"count": 1, "next": 0, "prev": 0},
				})
			}
		},
	})

	p := newTestVercelProvider(t, srv.URL)

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

func TestVercel_ListDomains_EmptyList(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"domains":    []any{},
				"pagination": map[string]any{"count": 0, "next": 0, "prev": 0},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(domains))
	}
}

func TestVercel_ListDomains_Unauthorized(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "unauthorized", "message": "Invalid token"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	_, err := p.ListDomains(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

// --- ListRecords tests ---

func TestVercel_ListRecords_HappyPath(t *testing.T) {
	prio := 10
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains/example.com/records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{
					testVercelRecordJSON("rec-1", "", "A", "1.2.3.4", 60, nil, ""),
					testVercelRecordJSON("rec-2", "www", "A", "1.2.3.4", 60, nil, ""),
					testVercelRecordJSON("rec-3", "", "MX", "mail.example.com", 3600, &prio, "mail server"),
				},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	records, err := p.ListRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Root record: empty name should become the domain name.
	if records[0].Name != "example.com" {
		t.Errorf("records[0].Name = %q, want %q", records[0].Name, "example.com")
	}
	// Subdomain record: should be FQDN.
	if records[1].Name != "www.example.com" {
		t.Errorf("records[1].Name = %q, want %q", records[1].Name, "www.example.com")
	}
	if records[0].Content != "1.2.3.4" {
		t.Errorf("records[0].Content = %q, want %q", records[0].Content, "1.2.3.4")
	}
	if records[2].Priority != 10 {
		t.Errorf("records[2].Priority = %d, want 10", records[2].Priority)
	}
	if records[2].Notes != "mail server" {
		t.Errorf("records[2].Notes = %q, want %q", records[2].Notes, "mail server")
	}
}

func TestVercel_ListRecords_NotFound(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains/notexist.com/records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "not_found", "message": "Domain not found"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	_, err := p.ListRecords(context.Background(), "notexist.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- GetRecord tests ---

func TestVercel_GetRecord_HappyPath(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains/example.com/records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{
					testVercelRecordJSON("rec-1", "", "A", "1.2.3.4", 60, nil, "test note"),
					testVercelRecordJSON("rec-2", "www", "CNAME", "example.com", 60, nil, ""),
				},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

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
		TTL:      60,
		Priority: 0,
		Notes:    "test note",
	}

	if diff := cmp.Diff(want, rec); diff != "" {
		t.Errorf("GetRecord mismatch (-want +got):\n%s", diff)
	}
}

func TestVercel_GetRecord_NotFound(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains/example.com/records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{
					testVercelRecordJSON("rec-1", "", "A", "1.2.3.4", 60, nil, ""),
				},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	_, err := p.GetRecord(context.Background(), "example.com", "rec-999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- CreateRecord tests ---

func TestVercel_CreateRecord_HappyPath(t *testing.T) {
	var capturedBody vercelCreateRecordBody
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"POST /v2/domains/example.com/records": func(w http.ResponseWriter, r *http.Request) {
			json.NewDecoder(r.Body).Decode(&capturedBody)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"uid":     "rec-new",
				"updated": 123,
			})
		},
		"GET /v5/domains/example.com/records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"records": []any{
					testVercelRecordJSON("rec-new", "www", "A", "5.6.7.8", 60, nil, ""),
				},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	rec, err := p.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Name:    "www",
		Type:    domain.RecordTypeA,
		Content: "5.6.7.8",
		TTL:     60,
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

	// Verify the request body.
	if capturedBody.Name != "www" {
		t.Errorf("request body name = %q, want %q", capturedBody.Name, "www")
	}
	if capturedBody.Value != "5.6.7.8" {
		t.Errorf("request body value = %q, want %q", capturedBody.Value, "5.6.7.8")
	}
}

func TestVercel_CreateRecord_Conflict(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"POST /v2/domains/example.com/records": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "conflict", "message": "Record already exists"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

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

func TestVercel_UpdateRecord_HappyPath(t *testing.T) {
	var capturedMethod string
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"PATCH /v1/domains/records/rec-1": func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		},
	})

	p := newTestVercelProvider(t, srv.URL)

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

func TestVercel_UpdateRecord_NotFound(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"PATCH /v1/domains/records/rec-999": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "not_found", "message": "Record not found"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

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

func TestVercel_DeleteRecord_HappyPath(t *testing.T) {
	var capturedPath string
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"DELETE /v2/domains/example.com/records/rec-1": func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	err := p.DeleteRecord(context.Background(), "example.com", "rec-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedPath != "/v2/domains/example.com/records/rec-1" {
		t.Errorf("expected path /v2/domains/example.com/records/rec-1, got %s", capturedPath)
	}
}

func TestVercel_DeleteRecord_NotFound(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"DELETE /v2/domains/example.com/records/rec-999": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "not_found", "message": "Record not found"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	err := p.DeleteRecord(context.Background(), "example.com", "rec-999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- CheckAvailability tests ---

func TestVercel_CheckAvailability_Available(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v1/registrar/domains/newdomain.com/availability": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"available": true})
		},
		"GET /v1/registrar/domains/newdomain.com/price": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"years":         1,
				"purchasePrice": 9.99,
				"renewalPrice":  12.99,
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	result, err := p.CheckAvailability(context.Background(), "newdomain.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := &domain.SearchResult{
		Domain:    "newdomain.com",
		Available: true,
		Price:     "9.99",
		Renewal:   "12.99",
	}

	if diff := cmp.Diff(want, result); diff != "" {
		t.Errorf("CheckAvailability mismatch (-want +got):\n%s", diff)
	}
}

func TestVercel_CheckAvailability_NotAvailable(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v1/registrar/domains/taken.com/availability": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"available": false})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	result, err := p.CheckAvailability(context.Background(), "taken.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Available {
		t.Error("expected Available = false")
	}
	if result.Price != "" {
		t.Errorf("expected empty Price for unavailable domain, got %q", result.Price)
	}
}

func TestVercel_CheckAvailability_PriceAsObject(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v1/registrar/domains/fancy.dev/availability": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"available": true})
		},
		"GET /v1/registrar/domains/fancy.dev/price": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"years":         1,
				"purchasePrice": map[string]any{"amount": 15.50, "currency": "USD"},
				"renewalPrice":  map[string]any{"amount": 18.00, "currency": "USD"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	result, err := p.CheckAvailability(context.Background(), "fancy.dev")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Price != "15.50" {
		t.Errorf("Price = %q, want %q", result.Price, "15.50")
	}
	if result.Renewal != "18.00" {
		t.Errorf("Renewal = %q, want %q", result.Renewal, "18.00")
	}
}

// --- Auth header tests ---

func TestVercel_BearerTokenSent(t *testing.T) {
	var capturedAuth string
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			capturedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"domains": []any{},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	p.ListDomains(context.Background())

	if capturedAuth != "Bearer test-token" {
		t.Errorf("expected Authorization = %q, got %q", "Bearer test-token", capturedAuth)
	}
}

// --- Team ID tests ---

func TestVercel_TeamIDAppended(t *testing.T) {
	var capturedQuery string
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"domains": []any{},
			})
		},
	})

	p := newTestVercelProviderWithTeam(t, srv.URL, "team_abc123")

	p.ListDomains(context.Background())

	if !strings.Contains(capturedQuery, "teamId=team_abc123") {
		t.Errorf("expected teamId in query, got %q", capturedQuery)
	}
}

// --- Error mapping tests ---

func TestVercel_ErrorMapping_Forbidden(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "forbidden", "message": "Insufficient permissions"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	_, err := p.ListDomains(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized for 403, got: %v", err)
	}
}

func TestVercel_ErrorMapping_RateLimited(t *testing.T) {
	srv := newVercelRouter(t, map[string]http.HandlerFunc{
		"GET /v5/domains": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "rate_limited", "message": "Too many requests"},
			})
		},
	})

	p := newTestVercelProvider(t, srv.URL)

	_, err := p.ListDomains(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Errorf("expected ErrRateLimited for 429, got: %v", err)
	}
}

// --- Registry tests ---

func TestVercel_Registry_RegisterAndGet(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	store.SetToken(vercelTokenStore, "vercel-test-token")

	RegisterVercel()

	p, err := Get("vercel", store)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.GetDisplayName() != "Vercel" {
		t.Errorf("GetDisplayName = %q, want %q", p.GetDisplayName(), "Vercel")
	}
}

func TestVercel_Registry_MissingToken(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	// No token set.

	RegisterVercel()

	_, err := Get("vercel", store)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}

func TestVercel_Registry_OptionalTeamID(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	store.SetToken(vercelTokenStore, "vercel-test-token")
	// Team ID not set — should still succeed.

	RegisterVercel()

	p, err := Get("vercel", store)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.GetDisplayName() != "Vercel" {
		t.Errorf("GetDisplayName = %q, want %q", p.GetDisplayName(), "Vercel")
	}
}
