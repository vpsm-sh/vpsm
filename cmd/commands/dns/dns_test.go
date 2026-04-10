package dns

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	dnsdomain "nathanbeddoewebdev/vpsm/internal/dns/domain"
	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

// --- Mock DNS provider ---

type mockDNSProvider struct {
	displayName   string
	domains       []dnsdomain.Domain
	records       []dnsdomain.Record
	createdRecord *dnsdomain.Record

	listDomainsErr error
	listRecordsErr error
	createErr      error
	updateErr      error
	deleteErr      error

	lastCreateOpts dnsdomain.CreateRecordOpts
	lastUpdateID   string
	lastDeleteID   string
}

func (m *mockDNSProvider) GetDisplayName() string { return m.displayName }

func (m *mockDNSProvider) ListDomains(_ context.Context) ([]dnsdomain.Domain, error) {
	return m.domains, m.listDomainsErr
}

func (m *mockDNSProvider) ListRecords(_ context.Context, _ string) ([]dnsdomain.Record, error) {
	return m.records, m.listRecordsErr
}

func (m *mockDNSProvider) GetRecord(_ context.Context, domain string, id string) (*dnsdomain.Record, error) {
	for _, r := range m.records {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, dnsdomain.ErrNotFound
}

func (m *mockDNSProvider) CreateRecord(_ context.Context, domain string, opts dnsdomain.CreateRecordOpts) (*dnsdomain.Record, error) {
	m.lastCreateOpts = opts
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.createdRecord != nil {
		return m.createdRecord, nil
	}
	rec := &dnsdomain.Record{
		ID:      "new-id",
		Domain:  domain,
		Type:    opts.Type,
		Content: opts.Content,
	}
	return rec, nil
}

func (m *mockDNSProvider) UpdateRecord(_ context.Context, _ string, id string, _ dnsdomain.UpdateRecordOpts) error {
	m.lastUpdateID = id
	return m.updateErr
}

func (m *mockDNSProvider) DeleteRecord(_ context.Context, _ string, id string) error {
	m.lastDeleteID = id
	return m.deleteErr
}

// registerMockDNSProvider resets the DNS registry and registers a mock provider factory.
func registerMockDNSProvider(t *testing.T, name string, mock *mockDNSProvider) {
	t.Helper()
	dnsproviders.Reset()
	t.Cleanup(dnsproviders.Reset)
	dnsproviders.Register(name, func(store auth.Store) (dnsdomain.Provider, error) {
		return mock, nil
	})
}

// execDNS runs the given dns subcommand args and returns stdout/stderr.
func execDNS(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	t.Setenv("VPSM_DISABLE_DNS_CACHE", "1")
	var outBuf, errBuf bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	cmd.Execute()
	return outBuf.String(), errBuf.String()
}

// --- domains tests ---

func TestDomainsCommand_ListsDomains(t *testing.T) {
	mock := &mockDNSProvider{
		displayName: "Mock",
		domains: []dnsdomain.Domain{
			{Name: "example.com", Status: "ACTIVE", TLD: "com", ExpireDate: "2025-01-01 00:00:00"},
			{Name: "mysite.io", Status: "ACTIVE", TLD: "io", ExpireDate: "2026-06-01 00:00:00"},
		},
	}
	registerMockDNSProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "domains", "--provider", "mock")
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	for _, want := range []string{"example.com", "mysite.io", "ACTIVE", "DOMAIN", "STATUS"} {
		if !contains(stdout, want) {
			t.Errorf("expected %q in output:\n%s", want, stdout)
		}
	}
}

func TestDomainsCommand_EmptyList(t *testing.T) {
	mock := &mockDNSProvider{displayName: "Mock"}
	registerMockDNSProvider(t, "mock", mock)

	stdout, _ := execDNS(t, "domains", "--provider", "mock")
	if !contains(stdout, "No domains found") {
		t.Errorf("expected 'No domains found' in output:\n%s", stdout)
	}
}

func TestDomainsCommand_ProviderError(t *testing.T) {
	mock := &mockDNSProvider{listDomainsErr: fmt.Errorf("api error")}
	registerMockDNSProvider(t, "mock", mock)

	_, stderr := execDNS(t, "domains", "--provider", "mock")
	if !contains(stderr, "api error") {
		t.Errorf("expected 'api error' in stderr:\n%s", stderr)
	}
}

// --- list tests ---

func TestListCommand_ListsRecords(t *testing.T) {
	mock := &mockDNSProvider{
		displayName: "Mock",
		records: []dnsdomain.Record{
			{ID: "101", Domain: "example.com", Name: "example.com", Type: dnsdomain.RecordTypeA, Content: "1.2.3.4", TTL: 600},
			{ID: "102", Domain: "example.com", Name: "www.example.com", Type: dnsdomain.RecordTypeCNAME, Content: "example.com", TTL: 600},
		},
	}
	registerMockDNSProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "list", "example.com", "--provider", "mock")
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	for _, want := range []string{"101", "102", "1.2.3.4", "CNAME", "ID", "NAME", "TYPE"} {
		if !contains(stdout, want) {
			t.Errorf("expected %q in output:\n%s", want, stdout)
		}
	}
}

func TestListCommand_EmptyList(t *testing.T) {
	mock := &mockDNSProvider{displayName: "Mock"}
	registerMockDNSProvider(t, "mock", mock)

	stdout, _ := execDNS(t, "list", "example.com", "--provider", "mock")
	if !contains(stdout, "No records found") {
		t.Errorf("expected 'No records found' in output:\n%s", stdout)
	}
}

func TestListCommand_FilterByType(t *testing.T) {
	mock := &mockDNSProvider{
		displayName: "Mock",
		records: []dnsdomain.Record{
			{ID: "101", Type: dnsdomain.RecordTypeA, Content: "1.2.3.4"},
			{ID: "102", Type: dnsdomain.RecordTypeMX, Content: "mail.example.com"},
		},
	}
	registerMockDNSProvider(t, "mock", mock)

	// Test uppercase filter
	stdout, _ := execDNS(t, "list", "example.com", "--provider", "mock", "--type", "A")
	if !contains(stdout, "101") {
		t.Errorf("expected record 101 in output:\n%s", stdout)
	}
	if contains(stdout, "102") {
		t.Errorf("expected record 102 to be filtered out:\n%s", stdout)
	}

	// Test lowercase filter
	stdout, _ = execDNS(t, "list", "example.com", "--provider", "mock", "--type", "a")
	if !contains(stdout, "101") {
		t.Errorf("expected record 101 in output with lowercase filter:\n%s", stdout)
	}
	if contains(stdout, "102") {
		t.Errorf("expected record 102 to be filtered out with lowercase filter:\n%s", stdout)
	}
}

func TestListCommand_ProviderError(t *testing.T) {
	mock := &mockDNSProvider{listRecordsErr: fmt.Errorf("network timeout")}
	registerMockDNSProvider(t, "mock", mock)

	_, stderr := execDNS(t, "list", "example.com", "--provider", "mock")
	if !contains(stderr, "network timeout") {
		t.Errorf("expected 'network timeout' in stderr:\n%s", stderr)
	}
}

// --- create tests ---

func TestCreateCommand_CreatesRecord(t *testing.T) {
	mock := &mockDNSProvider{
		displayName: "Mock",
		createdRecord: &dnsdomain.Record{
			ID:      "201",
			Domain:  "example.com",
			Name:    "www.example.com",
			Type:    dnsdomain.RecordTypeA,
			Content: "5.6.7.8",
		},
	}
	registerMockDNSProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "create", "example.com",
		"--provider", "mock",
		"--type", "A",
		"--name", "www",
		"--content", "5.6.7.8",
	)
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	if !contains(stdout, "201") {
		t.Errorf("expected record ID '201' in output:\n%s", stdout)
	}
}

func TestCreateCommand_MissingRequiredFlags(t *testing.T) {
	mock := &mockDNSProvider{displayName: "Mock"}
	registerMockDNSProvider(t, "mock", mock)

	// Missing --content
	_, stderr := execDNS(t, "create", "example.com", "--provider", "mock", "--type", "A")
	if !contains(stderr, "content") {
		t.Errorf("expected 'content' flag error in stderr:\n%s", stderr)
	}
}

func TestCreateCommand_ProviderError(t *testing.T) {
	mock := &mockDNSProvider{createErr: fmt.Errorf("duplicate record")}
	registerMockDNSProvider(t, "mock", mock)

	_, stderr := execDNS(t, "create", "example.com",
		"--provider", "mock",
		"--type", "A",
		"--content", "1.2.3.4",
	)
	if !contains(stderr, "duplicate record") {
		t.Errorf("expected 'duplicate record' in stderr:\n%s", stderr)
	}
}

// --- update tests ---

func TestUpdateCommand_UpdatesRecord(t *testing.T) {
	mock := &mockDNSProvider{displayName: "Mock"}
	registerMockDNSProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "update", "example.com", "101",
		"--provider", "mock",
		"--content", "9.9.9.9",
	)
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	if !contains(stdout, "101") {
		t.Errorf("expected record ID '101' in output:\n%s", stdout)
	}
	if mock.lastUpdateID != "101" {
		t.Errorf("lastUpdateID = %q, want %q", mock.lastUpdateID, "101")
	}
}

func TestUpdateCommand_ProviderError(t *testing.T) {
	mock := &mockDNSProvider{updateErr: fmt.Errorf("record not found")}
	registerMockDNSProvider(t, "mock", mock)

	_, stderr := execDNS(t, "update", "example.com", "999",
		"--provider", "mock",
		"--content", "1.1.1.1",
	)
	if !contains(stderr, "record not found") {
		t.Errorf("expected 'record not found' in stderr:\n%s", stderr)
	}
}

// --- delete tests ---

func TestDeleteCommand_DeletesRecord(t *testing.T) {
	mock := &mockDNSProvider{displayName: "Mock"}
	registerMockDNSProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "delete", "example.com", "101", "--provider", "mock")
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	if !contains(stdout, "101") {
		t.Errorf("expected record ID '101' in output:\n%s", stdout)
	}
	if mock.lastDeleteID != "101" {
		t.Errorf("lastDeleteID = %q, want %q", mock.lastDeleteID, "101")
	}
}

func TestDeleteCommand_ProviderError(t *testing.T) {
	mock := &mockDNSProvider{deleteErr: fmt.Errorf("record not found")}
	registerMockDNSProvider(t, "mock", mock)

	_, stderr := execDNS(t, "delete", "example.com", "999", "--provider", "mock")
	if !contains(stderr, "record not found") {
		t.Errorf("expected 'record not found' in stderr:\n%s", stderr)
	}
}

func TestDeleteCommand_UnknownProvider(t *testing.T) {
	dnsproviders.Reset()
	t.Cleanup(dnsproviders.Reset)

	_, stderr := execDNS(t, "delete", "example.com", "101", "--provider", "nonexistent")
	if !contains(stderr, "unknown provider") {
		t.Errorf("expected 'unknown provider' in stderr:\n%s", stderr)
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
