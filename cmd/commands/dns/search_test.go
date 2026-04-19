package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	dnsdomain "nathanbeddoewebdev/vpsm/internal/dns/domain"
	dnsproviders "nathanbeddoewebdev/vpsm/internal/dns/providers"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

// --- Mock search provider for CLI tests ---

type mockSearchDNSProvider struct {
	mockDNSProvider
	searchResult *dnsdomain.SearchResult
	searchErr    error
}

func (m *mockSearchDNSProvider) CheckAvailability(_ context.Context, _ string) (*dnsdomain.SearchResult, error) {
	return m.searchResult, m.searchErr
}

func registerMockSearchProvider(t *testing.T, name string, mock *mockSearchDNSProvider) {
	t.Helper()
	dnsproviders.Reset()
	t.Cleanup(dnsproviders.Reset)
	dnsproviders.Register(name, func(store auth.Store) (dnsdomain.Provider, error) {
		return mock, nil
	})
}

// --- search tests ---

func TestSearchCommand_Available(t *testing.T) {
	mock := &mockSearchDNSProvider{
		mockDNSProvider: mockDNSProvider{displayName: "Mock"},
		searchResult: &dnsdomain.SearchResult{
			Domain:    "newdomain.com",
			Available: true,
			Price:     "9.73",
			Renewal:   "9.73",
			Currency:  "USD",
		},
	}
	registerMockSearchProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "search", "newdomain.com", "--provider", "mock")
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	for _, want := range []string{"newdomain.com", "Yes", "9.73"} {
		if !contains(stdout, want) {
			t.Errorf("expected %q in output:\n%s", want, stdout)
		}
	}
}

func TestSearchCommand_Taken(t *testing.T) {
	mock := &mockSearchDNSProvider{
		mockDNSProvider: mockDNSProvider{displayName: "Mock"},
		searchResult: &dnsdomain.SearchResult{
			Domain:    "google.com",
			Available: false,
		},
	}
	registerMockSearchProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "search", "google.com", "--provider", "mock")
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	if !contains(stdout, "No") {
		t.Errorf("expected 'No' for taken domain in output:\n%s", stdout)
	}
}

func TestSearchCommand_ProviderDoesNotSupportSearch(t *testing.T) {
	mock := &mockDNSProvider{displayName: "Mock"}
	registerMockDNSProvider(t, "mock", mock)

	_, stderr := execDNS(t, "search", "example.com", "--provider", "mock")
	if !contains(stderr, "does not support") {
		t.Errorf("expected 'does not support' in stderr:\n%s", stderr)
	}
}

func TestSearchCommand_ProviderError(t *testing.T) {
	mock := &mockSearchDNSProvider{
		mockDNSProvider: mockDNSProvider{displayName: "Mock"},
		searchErr:       fmt.Errorf("api timeout"),
	}
	registerMockSearchProvider(t, "mock", mock)

	_, stderr := execDNS(t, "search", "example.com", "--provider", "mock")
	if !contains(stderr, "api timeout") {
		t.Errorf("expected 'api timeout' in stderr:\n%s", stderr)
	}
}

func TestSearchCommand_JSONOutput(t *testing.T) {
	mock := &mockSearchDNSProvider{
		mockDNSProvider: mockDNSProvider{displayName: "Mock"},
		searchResult: &dnsdomain.SearchResult{
			Domain:    "newdomain.com",
			Available: true,
			Premium:   false,
			Price:     "9.73",
			Renewal:   "9.73",
			Currency:  "USD",
		},
	}
	registerMockSearchProvider(t, "mock", mock)

	stdout, stderr := execDNS(t, "search", "newdomain.com", "--provider", "mock", "--output", "json")
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}

	var result dnsdomain.SearchResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput:\n%s", err, stdout)
	}

	if result.Domain != "newdomain.com" {
		t.Errorf("Domain = %q, want %q", result.Domain, "newdomain.com")
	}
	if !result.Available {
		t.Error("expected Available=true")
	}
	if result.Price != "9.73" {
		t.Errorf("Price = %q, want %q", result.Price, "9.73")
	}
}

func TestSearchCommand_MissingArg(t *testing.T) {
	mock := &mockSearchDNSProvider{
		mockDNSProvider: mockDNSProvider{displayName: "Mock"},
	}
	registerMockSearchProvider(t, "mock", mock)

	_, stderr := execDNS(t, "search", "--provider", "mock")
	if !contains(stderr, "accepts 1 arg") {
		t.Errorf("expected arg validation error in stderr:\n%s", stderr)
	}
}
