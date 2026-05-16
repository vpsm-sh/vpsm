package config

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"nathanbeddoewebdev/vpsm/internal/config"
	providernames "nathanbeddoewebdev/vpsm/internal/platform/providers/names"
)

// setupTestConfig points the config package at a temp file and returns cleanup.
func setupTestConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	config.SetPath(path)
	t.Cleanup(config.ResetPath)
	return path
}

// registerTestProvider registers a provider name for validation.
func registerTestProvider(t *testing.T, name string) {
	t.Helper()
	providernames.Reset()
	t.Cleanup(func() { providernames.Reset() })
	providernames.Register(name)
}

// execConfig creates the config command, wires up output buffers, runs with the
// given args, and returns what was written to stdout and stderr.
func execConfig(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	cmd.Execute()
	return outBuf.String(), errBuf.String()
}

func TestSet_DefaultProvider(t *testing.T) {
	setupTestConfig(t)
	registerTestProvider(t, "hetzner")

	stdout, stderr := execConfig(t, "set", "default-provider", "hetzner")

	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, `"hetzner"`) {
		t.Errorf("expected confirmation with provider name, got: %s", stdout)
	}

	// Verify it was persisted.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.DefaultProvider != "hetzner" {
		t.Errorf("expected DefaultProvider %q, got %q", "hetzner", cfg.DefaultProvider)
	}
}

func TestSet_DefaultProvider_UnknownProvider(t *testing.T) {
	setupTestConfig(t)
	registerTestProvider(t, "hetzner")

	_, stderr := execConfig(t, "set", "default-provider", "nonexistent")

	if !strings.Contains(stderr, "unknown provider") {
		t.Errorf("expected 'unknown provider' error, got: %s", stderr)
	}
}

func TestSet_UnknownKey(t *testing.T) {
	setupTestConfig(t)

	_, stderr := execConfig(t, "set", "bogus-key", "value")

	if !strings.Contains(stderr, "unknown configuration key") {
		t.Errorf("expected 'unknown configuration key' error, got: %s", stderr)
	}
}

func TestSet_DefaultProvider_CaseInsensitive(t *testing.T) {
	setupTestConfig(t)
	registerTestProvider(t, "hetzner")

	stdout, stderr := execConfig(t, "set", "default-provider", "HETZNER")

	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, `"hetzner"`) {
		t.Errorf("expected normalized provider name, got: %s", stdout)
	}
}

func TestSet_DefaultProvider_Vultr(t *testing.T) {
	setupTestConfig(t)
	registerTestProvider(t, "vultr")

	stdout, stderr := execConfig(t, "set", "default-provider", "vultr")

	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
	if !strings.Contains(stdout, `"vultr"`) {
		t.Errorf("expected confirmation with provider name, got: %s", stdout)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if cfg.DefaultProvider != "vultr" {
		t.Errorf("expected DefaultProvider %q, got %q", "vultr", cfg.DefaultProvider)
	}
}
