package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	appDir = "vpsm"
	dbFile = "vpsm.db"
)

var pathOverride string

// SetPath overrides the default database path. Intended for testing.
func SetPath(p string) { pathOverride = p }

// ResetPath clears the path override. Intended for testing.
func ResetPath() { pathOverride = "" }

// DefaultPath returns the default database path.
func DefaultPath() (string, error) {
	if pathOverride != "" {
		return pathOverride, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("database: unable to determine config directory: %w", err)
	}
	return filepath.Join(base, appDir, dbFile), nil
}

// Open opens a SQLite database at the provided path.
func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("database: failed to create directory %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("database: failed to open database: %w", err)
	}
	return db, nil
}
