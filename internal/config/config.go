// Package config handles persistent user configuration for vpsm.
//
// Configuration is stored as JSON at ~/.config/vpsm/config.json (or the
// platform-equivalent path returned by os.UserConfigDir).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	appDir   = "vpsm"
	fileName = "config.json"
)

// pathOverride, when non-empty, replaces the default config file path.
// Intended for testing. Use SetPath / ResetPath to manage.
var pathOverride string

// SetPath overrides the config file path. Intended for testing.
func SetPath(p string) { pathOverride = p }

// ResetPath clears the path override, reverting to the default. Intended for testing.
func ResetPath() { pathOverride = "" }

// Config holds user preferences that persist across invocations.
type Config struct {
	DefaultProvider string `json:"default_provider,omitempty"`
	DNSProvider     string `json:"dns_provider,omitempty"`
}

// Path returns the absolute path to the config file.
// If SetPath has been called, that value is returned instead.
// Otherwise it uses os.UserConfigDir which resolves to
// ~/Library/Application Support on macOS, ~/.config on Linux, and
// %AppData% on Windows.
func Path() (string, error) {
	if pathOverride != "" {
		return pathOverride, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: unable to determine config directory: %w", err)
	}
	return filepath.Join(base, appDir, fileName), nil
}

// Load reads the config file from disk and returns the parsed Config.
// If the file does not exist, a zero-value Config is returned (not an error).
func Load() (*Config, error) {
	return loadFrom("")
}

// loadFrom reads the config from the given path. If path is empty, the
// default Path() is used. Exported only for testing via LoadFrom.
func loadFrom(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = Path()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("config: failed to read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: failed to parse %s: %w", path, err)
	}

	return &cfg, nil
}

// Save writes the config to disk, creating the parent directory if needed.
func (c *Config) Save() error {
	return c.saveTo("")
}

// saveTo writes the config to the given path. If path is empty, the
// default Path() is used.
func (c *Config) saveTo(path string) error {
	if path == "" {
		var err error
		path, err = Path()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: failed to create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("config: failed to marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: failed to write %s: %w", path, err)
	}

	return nil
}

// LoadFrom reads the config from the given path. Intended for testing.
func LoadFrom(path string) (*Config, error) {
	return loadFrom(path)
}

// SaveTo writes the config to the given path. Intended for testing.
func (c *Config) SaveTo(path string) error {
	return c.saveTo(path)
}
