package server

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

// mockProvider implements domain.Provider for CLI testing.
type mockProvider struct {
	displayName string
	servers     []domain.Server
	listErr     error
}

func (m *mockProvider) GetDisplayName() string { return m.displayName }
func (m *mockProvider) CreateServer(_ context.Context, opts domain.CreateServerOpts) (*domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockProvider) DeleteServer(_ context.Context, id string) error {
	return fmt.Errorf("not implemented")
}
func (m *mockProvider) GetServer(_ context.Context, id string) (*domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockProvider) ListServers(_ context.Context) ([]domain.Server, error) {
	return m.servers, m.listErr
}
func (m *mockProvider) StartServer(_ context.Context, _ string) (*domain.ActionStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *mockProvider) StopServer(_ context.Context, _ string) (*domain.ActionStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

// registerMockProvider resets the global registry and registers a mock provider factory.
func registerMockProvider(t *testing.T, name string, mock *mockProvider) {
	t.Helper()
	providers.Reset()
	t.Cleanup(func() { providers.Reset() })
	providers.Register(name, func(store auth.Store) (domain.Provider, error) {
		return mock, nil
	})
}

// execList creates the server command, wires up output buffers, runs "list --provider <provider>",
// and returns what was written to stdout and stderr.
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

// assertContainsAll verifies that output contains every expected substring.
func assertContainsAll(t *testing.T, output string, label string, expected []string) {
	t.Helper()
	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in %s output:\n%s", want, label, output)
		}
	}
}

func TestListCommand_DisplaysServers(t *testing.T) {
	mock := &mockProvider{
		displayName: "Mock",
		servers: []domain.Server{
			{
				ID:         "42",
				Name:       "web-server",
				Status:     "running",
				CreatedAt:  time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
				PublicIPv4: "1.2.3.4",
				Region:     "fsn1",
				ServerType: "cpx11",
				Image:      "ubuntu-24.04",
				Provider:   "mock",
			},
			{
				ID:         "99",
				Name:       "db-server",
				Status:     "stopped",
				CreatedAt:  time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC),
				PublicIPv4: "5.6.7.8",
				Region:     "nbg1",
				ServerType: "cpx22",
				Image:      "debian-12",
				Provider:   "mock",
			},
		},
	}

	registerMockProvider(t, "mock", mock)

	stdout, _ := execList(t, "mock")

	// Verify table headers and both server rows in one pass.
	assertContainsAll(t, stdout, "stdout", []string{
		// Headers
		"ID", "NAME", "STATUS", "PUBLIC IPv4",
		// First server
		"42", "web-server", "running", "1.2.3.4", "fsn1", "cpx11", "ubuntu-24.04",
		// Second server
		"99", "db-server", "stopped", "5.6.7.8", "nbg1", "cpx22", "debian-12",
	})

	// Verify both rows appear on separate lines (header + separator + 2 data rows = 4).
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines (header + separator + 2 rows), got %d:\n%s", len(lines), stdout)
	}
}

func TestListCommand_VultrProviderName(t *testing.T) {
	mock := &mockProvider{
		displayName: "Vultr",
		servers: []domain.Server{
			{
				ID:         "instance-1",
				Name:       "web-1",
				Status:     "running",
				PublicIPv4: "203.0.113.10",
				Region:     "ewr",
				ServerType: "vc2-1c-2gb",
				Image:      "Ubuntu 24.04 LTS x64",
				Provider:   "vultr",
			},
		},
	}

	registerMockProvider(t, "vultr", mock)

	stdout, stderr := execList(t, "vultr")

	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	assertContainsAll(t, stdout, "stdout", []string{
		"instance-1", "web-1", "running", "203.0.113.10", "ewr", "vc2-1c-2gb",
	})
}

func TestListCommand_EmptyList(t *testing.T) {
	mock := &mockProvider{
		displayName: "Mock",
		servers:     []domain.Server{},
	}

	registerMockProvider(t, "mock", mock)

	stdout, _ := execList(t, "mock")

	if !strings.Contains(stdout, "No servers found") {
		t.Errorf("expected 'No servers found' message, got:\n%s", stdout)
	}
}

func TestListCommand_ProviderListError(t *testing.T) {
	mock := &mockProvider{
		displayName: "Mock",
		listErr:     fmt.Errorf("api connection failed"),
	}

	registerMockProvider(t, "mock", mock)

	_, stderr := execList(t, "mock")

	if !strings.Contains(stderr, "api connection failed") {
		t.Errorf("expected error 'api connection failed' on stderr, got:\n%s", stderr)
	}
}

func TestListCommand_UnknownProvider(t *testing.T) {
	providers.Reset()
	t.Cleanup(func() { providers.Reset() })

	_, stderr := execList(t, "nonexistent")

	if !strings.Contains(stderr, "unknown provider") {
		t.Errorf("expected 'unknown provider' error on stderr, got:\n%s", stderr)
	}
}
