package providers

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
	"nathanbeddoewebdev/vpsm/internal/services/auth"

	"github.com/google/go-cmp/cmp"
)

// --- Test helpers ---

// newTestPorkbunProvider creates a PorkbunProvider pointed at the given test server.
func newTestPorkbunProvider(t *testing.T, serverURL string) *PorkbunProvider {
	t.Helper()
	p := NewPorkbunProvider("test-api-key", "test-secret-key")
	p.baseURL = serverURL
	return p
}

// porkbunSuccess returns a minimal success response body.
func porkbunSuccess(extra map[string]any) map[string]any {
	m := map[string]any{"status": "SUCCESS"}
	maps.Copy(m, extra)
	return m
}

// porkbunError returns an error response body.
func porkbunError(message string) map[string]any {
	return map[string]any{
		"status":  "ERROR",
		"message": message,
	}
}

// newStaticServer creates an httptest.Server that always returns the given JSON.
func newStaticServer(t *testing.T, body any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Fatalf("failed to encode test response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// testRecordJSON returns a sample Porkbun API record object.
func testRecordJSON(id, name, typ, content, ttl, prio string) map[string]any {
	return map[string]any{
		"id":      id,
		"name":    name,
		"type":    typ,
		"content": content,
		"ttl":     ttl,
		"prio":    prio,
		"notes":   "",
	}
}

// --- ListDomains tests ---

func TestListDomains_HappyPath(t *testing.T) {
	body := porkbunSuccess(map[string]any{
		"domains": []any{
			map[string]any{
				"domain":     "example.com",
				"status":     "ACTIVE",
				"tld":        "com",
				"createDate": "2022-01-01 00:00:00",
				"expireDate": "2025-01-01 00:00:00",
			},
			map[string]any{
				"domain":     "another.io",
				"status":     "ACTIVE",
				"tld":        "io",
				"createDate": "2023-06-15 00:00:00",
				"expireDate": "2024-06-15 00:00:00",
			},
		},
	})

	srv := newStaticServer(t, body)
	p := newTestPorkbunProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}

	want := []domain.Domain{
		{Name: "example.com", Status: "ACTIVE", TLD: "com", CreateDate: "2022-01-01 00:00:00", ExpireDate: "2025-01-01 00:00:00"},
		{Name: "another.io", Status: "ACTIVE", TLD: "io", CreateDate: "2023-06-15 00:00:00", ExpireDate: "2024-06-15 00:00:00"},
	}

	if diff := cmp.Diff(want, domains); diff != "" {
		t.Errorf("ListDomains mismatch (-want +got):\n%s", diff)
	}
}

func TestListDomains_EmptyList(t *testing.T) {
	srv := newStaticServer(t, porkbunSuccess(map[string]any{
		"domains": []any{},
	}))
	p := newTestPorkbunProvider(t, srv.URL)

	domains, err := p.ListDomains(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(domains))
	}
}

func TestListDomains_Unauthorized(t *testing.T) {
	srv := newStaticServer(t, porkbunError("Invalid API key or secret."))
	p := newTestPorkbunProvider(t, srv.URL)

	_, err := p.ListDomains(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

// --- ListRecords tests ---

func TestListRecords_HappyPath(t *testing.T) {
	body := porkbunSuccess(map[string]any{
		"records": []any{
			testRecordJSON("101", "example.com", "A", "1.2.3.4", "600", "0"),
			testRecordJSON("102", "www.example.com", "A", "1.2.3.4", "600", "0"),
			testRecordJSON("103", "example.com", "MX", "mail.example.com", "3600", "10"),
		},
	})

	srv := newStaticServer(t, body)
	p := newTestPorkbunProvider(t, srv.URL)

	records, err := p.ListRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	if records[0].ID != "101" {
		t.Errorf("records[0].ID = %q, want %q", records[0].ID, "101")
	}
	if records[2].Priority != 10 {
		t.Errorf("records[2].Priority = %d, want 10", records[2].Priority)
	}
}

func TestListRecords_NotFound(t *testing.T) {
	srv := newStaticServer(t, porkbunError("Domain does not exist."))
	p := newTestPorkbunProvider(t, srv.URL)

	_, err := p.ListRecords(context.Background(), "notexist.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- GetRecord tests ---

func TestGetRecord_HappyPath(t *testing.T) {
	body := porkbunSuccess(map[string]any{
		"records": []any{
			testRecordJSON("101", "example.com", "A", "1.2.3.4", "600", "0"),
		},
	})

	srv := newStaticServer(t, body)
	p := newTestPorkbunProvider(t, srv.URL)

	rec, err := p.GetRecord(context.Background(), "example.com", "101")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	want := &domain.Record{
		ID:       "101",
		Domain:   "example.com",
		Name:     "example.com",
		Type:     domain.RecordTypeA,
		Content:  "1.2.3.4",
		TTL:      600,
		Priority: 0,
		Notes:    "",
	}

	if diff := cmp.Diff(want, rec); diff != "" {
		t.Errorf("GetRecord mismatch (-want +got):\n%s", diff)
	}
}

func TestGetRecord_EmptyRecords(t *testing.T) {
	srv := newStaticServer(t, porkbunSuccess(map[string]any{
		"records": []any{},
	}))
	p := newTestPorkbunProvider(t, srv.URL)

	_, err := p.GetRecord(context.Background(), "example.com", "999")
	if err == nil {
		t.Fatal("expected error for empty records, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- CreateRecord tests ---

func TestCreateRecord_HappyPath(t *testing.T) {
	// First call returns the create response, second returns the record.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		var body any
		if callCount == 1 {
			// POST /dns/create/example.com
			body = porkbunSuccess(map[string]any{"id": 201})
		} else {
			// POST /dns/retrieve/example.com/201
			body = porkbunSuccess(map[string]any{
				"records": []any{
					testRecordJSON("201", "www.example.com", "A", "5.6.7.8", "600", "0"),
				},
			})
		}
		json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	p := newTestPorkbunProvider(t, srv.URL)

	rec, err := p.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Name:    "www",
		Type:    domain.RecordTypeA,
		Content: "5.6.7.8",
		TTL:     600,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rec.ID != "201" {
		t.Errorf("rec.ID = %q, want %q", rec.ID, "201")
	}
	if rec.Content != "5.6.7.8" {
		t.Errorf("rec.Content = %q, want %q", rec.Content, "5.6.7.8")
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (create + fetch), got %d", callCount)
	}
}

func TestCreateRecord_APIError(t *testing.T) {
	srv := newStaticServer(t, porkbunError("Record already exists."))
	p := newTestPorkbunProvider(t, srv.URL)

	_, err := p.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "1.1.1.1",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- UpdateRecord tests ---

func TestUpdateRecord_HappyPath(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(porkbunSuccess(nil))
	}))
	t.Cleanup(srv.Close)

	p := newTestPorkbunProvider(t, srv.URL)

	err := p.UpdateRecord(context.Background(), "example.com", "101", domain.UpdateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "9.9.9.9",
		TTL:     1800,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedPath != "/dns/edit/example.com/101" {
		t.Errorf("expected path /dns/edit/example.com/101, got %s", capturedPath)
	}
}

func TestUpdateRecord_NotFound(t *testing.T) {
	srv := newStaticServer(t, porkbunError("Record does not exist."))
	p := newTestPorkbunProvider(t, srv.URL)

	err := p.UpdateRecord(context.Background(), "example.com", "999", domain.UpdateRecordOpts{
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

func TestDeleteRecord_HappyPath(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(porkbunSuccess(nil))
	}))
	t.Cleanup(srv.Close)

	p := newTestPorkbunProvider(t, srv.URL)

	err := p.DeleteRecord(context.Background(), "example.com", "101")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if capturedPath != "/dns/delete/example.com/101" {
		t.Errorf("expected path /dns/delete/example.com/101, got %s", capturedPath)
	}
}

func TestDeleteRecord_NotFound(t *testing.T) {
	srv := newStaticServer(t, porkbunError("Record does not exist."))
	p := newTestPorkbunProvider(t, srv.URL)

	err := p.DeleteRecord(context.Background(), "example.com", "999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- Registry tests ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	store.SetToken(porkbunAPIKeyStore, "ak")
	store.SetToken(porkbunSecretStore, "sk")

	RegisterPorkbun()

	p, err := Get("porkbun", store)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.GetDisplayName() != "Porkbun" {
		t.Errorf("GetDisplayName = %q, want %q", p.GetDisplayName(), "Porkbun")
	}
}

func TestRegistry_MissingAPIKey(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	// Only set secret, not api key
	store.SetToken(porkbunSecretStore, "sk")

	RegisterPorkbun()

	_, err := Get("porkbun", store)
	if err == nil {
		t.Fatal("expected error for missing api key, got nil")
	}
}

func TestRegistry_MissingSecretKey(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	store := auth.NewMockStore()
	// Only set api key, not secret
	store.SetToken(porkbunAPIKeyStore, "ak")

	RegisterPorkbun()

	_, err := Get("porkbun", store)
	if err == nil {
		t.Fatal("expected error for missing secret key, got nil")
	}
}

func TestRegistry_UnknownProvider(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	_, err := Get("nonexistent", auth.NewMockStore())
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}
