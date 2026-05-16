package server

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"nathanbeddoewebdev/vpsm/internal/server/domain"
	"nathanbeddoewebdev/vpsm/internal/server/providers"
	"nathanbeddoewebdev/vpsm/internal/services/auth"
)

// sshMockProvider implements domain.Provider with configurable GetServer behavior.
// Used to test SSH command validation logic without executing actual SSH.
type sshMockProvider struct {
	displayName  string
	getServer    *domain.Server
	getServerErr error
}

func (m *sshMockProvider) GetDisplayName() string { return m.displayName }
func (m *sshMockProvider) CreateServer(_ context.Context, _ domain.CreateServerOpts) (*domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *sshMockProvider) DeleteServer(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented")
}
func (m *sshMockProvider) GetServer(_ context.Context, id string) (*domain.Server, error) {
	if m.getServerErr != nil {
		return nil, m.getServerErr
	}
	return m.getServer, nil
}
func (m *sshMockProvider) ListServers(_ context.Context) ([]domain.Server, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *sshMockProvider) StartServer(_ context.Context, _ string) (*domain.ActionStatus, error) {
	return nil, fmt.Errorf("not implemented")
}
func (m *sshMockProvider) StopServer(_ context.Context, _ string) (*domain.ActionStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

// --- Helpers ---

func registerSSHMockProvider(t *testing.T, name string, mock *sshMockProvider) {
	t.Helper()
	providers.Reset()
	t.Cleanup(func() { providers.Reset() })
	providers.Register(name, func(store auth.Store) (domain.Provider, error) {
		return mock, nil
	})
}

// execSSH creates the server command, wires up output buffers, runs "ssh --provider <provider> [flags...]",
// and returns what was written to stdout and stderr.
// Note: This will never actually execute SSH since we can't test syscall.Exec,
// but it will test the validation logic up to that point.
func execSSH(t *testing.T, providerName string, extraArgs ...string) (stdout, stderr string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	args := append([]string{"ssh", "--provider", providerName}, extraArgs...)
	cmd.SetArgs(args)
	cmd.Execute()
	return outBuf.String(), errBuf.String()
}

// --- Validation tests ---

func TestSSHCommand_MissingID(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock")

	if !strings.Contains(stderr, "server ID required") {
		t.Errorf("expected 'server ID required' error on stderr, got:\n%s", stderr)
	}
}

func TestSSHCommand_PositionalID(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:         "42",
			Name:       "test-server",
			Status:     "off",
			PublicIPv4: "203.0.113.42",
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "42")

	if !strings.Contains(stderr, "server 42 is not running") {
		t.Errorf("expected 'server 42 is not running' error, got:\n%s", stderr)
	}
}

func TestSSHCommand_UserAtID(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:         "42",
			Name:       "test-server",
			Status:     "off",
			PublicIPv4: "203.0.113.42",
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "ubuntu@42")

	if !strings.Contains(stderr, "server 42 is not running") {
		t.Errorf("expected positional arg parsed as user@id, got:\n%s", stderr)
	}
}

func TestSSHCommand_UserFlag(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:         "42",
			Name:       "test-server",
			Status:     "off",
			PublicIPv4: "203.0.113.42",
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "--user", "ubuntu", "42")

	if !strings.Contains(stderr, "server 42 is not running") {
		t.Errorf("expected --user flag accepted, got:\n%s", stderr)
	}
}

func TestSSHCommand_UserFlagShorthand(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:         "42",
			Name:       "test-server",
			Status:     "off",
			PublicIPv4: "203.0.113.42",
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "-U", "ubuntu", "42")

	if !strings.Contains(stderr, "server 42 is not running") {
		t.Errorf("expected -U shorthand accepted, got:\n%s", stderr)
	}
}

func TestSSHCommand_DeprecatedIDFlag(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:         "42",
			Name:       "test-server",
			Status:     "off",
			PublicIPv4: "203.0.113.42",
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "--id", "42")

	if !strings.Contains(stderr, "server 42 is not running") {
		t.Errorf("expected deprecated --id flag to work, got:\n%s", stderr)
	}
}

func TestSSHCommand_ServerNotRunning(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:         "42",
			Name:       "test-server",
			Status:     "off",
			PublicIPv4: "203.0.113.42",
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "--id", "42")

	if !strings.Contains(stderr, "not running") {
		t.Errorf("expected 'not running' error on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "status: off") {
		t.Errorf("expected status displayed on stderr, got:\n%s", stderr)
	}
}

func TestSSHCommand_NoPublicIP(t *testing.T) {
	mock := &sshMockProvider{
		displayName: "Mock",
		getServer: &domain.Server{
			ID:     "42",
			Name:   "test-server",
			Status: "running",
			// No PublicIPv4 or PublicIPv6 set
		},
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "--id", "42")

	if !strings.Contains(stderr, "no public IP") {
		t.Errorf("expected 'no public IP' error on stderr, got:\n%s", stderr)
	}
}

func TestSSHCommand_GetServerError(t *testing.T) {
	mock := &sshMockProvider{
		displayName:  "Mock",
		getServerErr: fmt.Errorf("server not found"),
	}

	registerSSHMockProvider(t, "mock", mock)

	_, stderr := execSSH(t, "mock", "--id", "999")

	if !strings.Contains(stderr, "server not found") {
		t.Errorf("expected 'server not found' on stderr, got:\n%s", stderr)
	}
}

func TestSSHCommand_UnknownProvider(t *testing.T) {
	providers.Reset()
	t.Cleanup(func() { providers.Reset() })

	_, stderr := execSSH(t, "nonexistent", "--id", "42")

	if !strings.Contains(stderr, "unknown provider") {
		t.Errorf("expected 'unknown provider' error on stderr, got:\n%s", stderr)
	}
}

func TestParseSSHTarget(t *testing.T) {
	tests := []struct {
		target   string
		wantUser string
		wantID   string
	}{
		{"42", "", "42"},
		{"ubuntu@42", "ubuntu", "42"},
		{"user@name@42", "user", "name@42"},
		{"root@10", "root", "10"},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			gotUser, gotID := parseSSHTarget(tt.target)
			if gotUser != tt.wantUser {
				t.Errorf("user = %q, want %q", gotUser, tt.wantUser)
			}
			if gotID != tt.wantID {
				t.Errorf("id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}
