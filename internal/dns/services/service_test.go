package services

import (
	"context"
	"errors"
	"testing"

	"nathanbeddoewebdev/vpsm/internal/dns/domain"
)

// --- Mock provider ---

type mockProvider struct {
	domains        []domain.Domain
	records        []domain.Record
	listDomainsErr error
	listRecordsErr error
	getRecordErr   error
	createErr      error
	updateErr      error
	deleteErr      error

	// Capture arguments for assertion.
	lastCreateOpts domain.CreateRecordOpts
	lastUpdateOpts domain.UpdateRecordOpts
	lastDomain     string
	lastID         string
}

func (m *mockProvider) GetDisplayName() string { return "Mock" }

func (m *mockProvider) ListDomains(_ context.Context) ([]domain.Domain, error) {
	return m.domains, m.listDomainsErr
}

func (m *mockProvider) ListRecords(_ context.Context, d string) ([]domain.Record, error) {
	m.lastDomain = d
	return m.records, m.listRecordsErr
}

func (m *mockProvider) GetRecord(_ context.Context, d string, id string) (*domain.Record, error) {
	m.lastDomain = d
	m.lastID = id
	if m.getRecordErr != nil {
		return nil, m.getRecordErr
	}
	if len(m.records) == 0 {
		return nil, domain.ErrNotFound
	}
	return &m.records[0], nil
}

func (m *mockProvider) CreateRecord(_ context.Context, d string, opts domain.CreateRecordOpts) (*domain.Record, error) {
	m.lastDomain = d
	m.lastCreateOpts = opts
	if m.createErr != nil {
		return nil, m.createErr
	}
	rec := domain.Record{
		ID:      "new-id",
		Domain:  d,
		Name:    opts.Name,
		Type:    opts.Type,
		Content: opts.Content,
		TTL:     opts.TTL,
	}
	return &rec, nil
}

func (m *mockProvider) UpdateRecord(_ context.Context, d string, id string, opts domain.UpdateRecordOpts) error {
	m.lastDomain = d
	m.lastID = id
	m.lastUpdateOpts = opts
	return m.updateErr
}

func (m *mockProvider) DeleteRecord(_ context.Context, d string, id string) error {
	m.lastDomain = d
	m.lastID = id
	return m.deleteErr
}

// --- ListRecords tests ---

func TestService_ListRecords_NormalizesDomain(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	_, _ = svc.ListRecords(context.Background(), "  EXAMPLE.COM.  ")

	if mock.lastDomain != "example.com" {
		t.Errorf("lastDomain = %q, want %q", mock.lastDomain, "example.com")
	}
}

func TestService_ListRecords_EmptyDomain(t *testing.T) {
	svc := New(&mockProvider{})
	_, err := svc.ListRecords(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty domain, got nil")
	}
}

func TestService_ListRecords_PropagatesProviderError(t *testing.T) {
	want := errors.New("provider down")
	svc := New(&mockProvider{listRecordsErr: want})

	_, err := svc.ListRecords(context.Background(), "example.com")
	if !errors.Is(err, want) {
		t.Errorf("expected provider error to propagate, got: %v", err)
	}
}

// --- CreateRecord tests ---

func TestService_CreateRecord_AppliesDefaultTTL(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "1.2.3.4",
		TTL:     0, // should become DefaultTTL
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.lastCreateOpts.TTL != DefaultTTL {
		t.Errorf("TTL = %d, want %d", mock.lastCreateOpts.TTL, DefaultTTL)
	}
}

func TestService_CreateRecord_StripsFQDNSubdomain(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Name:    "www.example.com", // FQDN — should be stripped to "www"
		Type:    domain.RecordTypeA,
		Content: "1.2.3.4",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.lastCreateOpts.Name != "www" {
		t.Errorf("Name = %q, want %q", mock.lastCreateOpts.Name, "www")
	}
}

func TestService_CreateRecord_InvalidRecordType(t *testing.T) {
	svc := New(&mockProvider{})

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    "BOGUS",
		Content: "1.2.3.4",
	})
	if err == nil {
		t.Fatal("expected error for invalid record type, got nil")
	}
}

func TestService_CreateRecord_InvalidARecordContent(t *testing.T) {
	svc := New(&mockProvider{})

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "not-an-ip",
	})
	if err == nil {
		t.Fatal("expected error for non-IP A record content, got nil")
	}
}

func TestService_CreateRecord_InvalidAAAAContent(t *testing.T) {
	svc := New(&mockProvider{})

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeAAAA,
		Content: "1.2.3.4", // IPv4 is not valid for AAAA
	})
	if err == nil {
		t.Fatal("expected error for IPv4 in AAAA record, got nil")
	}
}

func TestService_CreateRecord_ValidAAAA(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeAAAA,
		Content: "2001:db8::1",
	})
	if err != nil {
		t.Fatalf("expected no error for valid AAAA, got %v", err)
	}
}

func TestService_CreateRecord_NormalizesDomain(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	_, _ = svc.CreateRecord(context.Background(), "EXAMPLE.COM.", domain.CreateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "1.2.3.4",
	})
	if mock.lastDomain != "example.com" {
		t.Errorf("lastDomain = %q, want %q", mock.lastDomain, "example.com")
	}
}

func TestService_CreateRecord_EmptyContent(t *testing.T) {
	svc := New(&mockProvider{})

	_, err := svc.CreateRecord(context.Background(), "example.com", domain.CreateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "",
	})
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

// --- UpdateRecord tests ---

func TestService_UpdateRecord_PassesNormalizedInputs(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	err := svc.UpdateRecord(context.Background(), "EXAMPLE.COM.", "101", domain.UpdateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "9.9.9.9",
		Name:    "www.example.com", // FQDN subdomain should be stripped
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.lastDomain != "example.com" {
		t.Errorf("lastDomain = %q, want %q", mock.lastDomain, "example.com")
	}
	if mock.lastUpdateOpts.Name != "www" {
		t.Errorf("opts.Name = %q, want %q", mock.lastUpdateOpts.Name, "www")
	}
}

func TestService_UpdateRecord_EmptyID(t *testing.T) {
	svc := New(&mockProvider{})

	err := svc.UpdateRecord(context.Background(), "example.com", "", domain.UpdateRecordOpts{
		Type:    domain.RecordTypeA,
		Content: "1.1.1.1",
	})
	if err == nil {
		t.Fatal("expected error for empty ID, got nil")
	}
}

// --- DeleteRecord tests ---

func TestService_DeleteRecord_PassesNormalizedDomain(t *testing.T) {
	mock := &mockProvider{}
	svc := New(mock)

	err := svc.DeleteRecord(context.Background(), "EXAMPLE.COM.", "101")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if mock.lastDomain != "example.com" {
		t.Errorf("lastDomain = %q, want %q", mock.lastDomain, "example.com")
	}
	if mock.lastID != "101" {
		t.Errorf("lastID = %q, want %q", mock.lastID, "101")
	}
}

func TestService_DeleteRecord_EmptyDomain(t *testing.T) {
	svc := New(&mockProvider{})

	err := svc.DeleteRecord(context.Background(), "", "101")
	if err == nil {
		t.Fatal("expected error for empty domain, got nil")
	}
}

func TestService_DeleteRecord_EmptyID(t *testing.T) {
	svc := New(&mockProvider{})

	err := svc.DeleteRecord(context.Background(), "example.com", "")
	if err == nil {
		t.Fatal("expected error for empty ID, got nil")
	}
}

// --- normalizeDomain tests ---

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

// --- normalizeSubdomain tests ---

func TestNormalizeSubdomain(t *testing.T) {
	cases := []struct {
		sub    string
		domain string
		want   string
	}{
		{"www", "example.com", "www"},
		{"www.example.com", "example.com", "www"},
		{"example.com", "example.com", ""},
		{"", "example.com", ""},
		{"WWW", "example.com", "www"},
		{"mail.example.com.", "example.com", "mail"},
	}

	for _, c := range cases {
		got := normalizeSubdomain(c.sub, c.domain)
		if got != c.want {
			t.Errorf("normalizeSubdomain(%q, %q) = %q, want %q", c.sub, c.domain, got, c.want)
		}
	}
}
