// Package serverprefs provides persistent storage for per-server user preferences.
//
// Preferences such as SSH usernames are stored keyed by (provider, server_id)
// so that different servers can have different defaults.
//
// Storage is backed by a SQLite database at ~/.config/vpsm/vpsm.db
// (shared with actionstore, separate table).
package serverprefs

import (
	"database/sql"
	"fmt"
	"time"

	"nathanbeddoewebdev/vpsm/internal/database"
)

// SetPath overrides the database path. Intended for testing.
func SetPath(p string) { database.SetPath(p) }

// ResetPath clears the path override, reverting to the default. Intended for testing.
func ResetPath() { database.ResetPath() }

// Repository defines the persistence interface for server preferences.
type Repository interface {
	// Get returns preferences for a (provider, serverID) pair, or nil if not found.
	Get(provider, serverID string) (*ServerPrefs, error)

	// Save upserts preferences for a server.
	Save(prefs *ServerPrefs) error

	// Close releases database resources.
	Close() error
}

// SQLiteRepository implements Repository backed by a local SQLite database.
type SQLiteRepository struct {
	db *sql.DB
}

// DefaultPath returns the default database path.
func DefaultPath() (string, error) {
	path, err := database.DefaultPath()
	if err != nil {
		return "", fmt.Errorf("serverprefs: %w", err)
	}
	return path, nil
}

// Open creates or opens the repository at the default path.
func Open() (*SQLiteRepository, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return OpenAt(path)
}

// OpenAt creates or opens a SQLite database at the given path.
// The parent directory is created if it does not exist.
func OpenAt(path string) (*SQLiteRepository, error) {
	db, err := database.Open(path)
	if err != nil {
		return nil, fmt.Errorf("serverprefs: %w", err)
	}

	r := &SQLiteRepository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return r, nil
}

// migrate creates the server_prefs table if it doesn't exist.
func (r *SQLiteRepository) migrate() error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS server_prefs (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			provider   TEXT NOT NULL,
			server_id  TEXT NOT NULL,
			ssh_user   TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(provider, server_id)
		);
	`
	if _, err := r.db.Exec(ddl); err != nil {
		return fmt.Errorf("serverprefs: migration failed: %w", err)
	}
	return nil
}

// Get returns preferences for a (provider, serverID) pair, or nil if not found.
func (r *SQLiteRepository) Get(provider, serverID string) (*ServerPrefs, error) {
	row := r.db.QueryRow(`
		SELECT id, provider, server_id, ssh_user, updated_at
		FROM server_prefs WHERE provider = ? AND server_id = ?`,
		provider, serverID)

	var prefs ServerPrefs
	var updatedStr string
	err := row.Scan(&prefs.ID, &prefs.Provider, &prefs.ServerID, &prefs.SSHUser, &updatedStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("serverprefs: query failed: %w", err)
	}
	prefs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return &prefs, nil
}

// Save upserts preferences for a server.
func (r *SQLiteRepository) Save(prefs *ServerPrefs) error {
	prefs.UpdatedAt = time.Now().UTC()

	result, err := r.db.Exec(`
		INSERT INTO server_prefs (provider, server_id, ssh_user, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider, server_id) DO UPDATE SET
			ssh_user = excluded.ssh_user,
			updated_at = excluded.updated_at`,
		prefs.Provider, prefs.ServerID, prefs.SSHUser, prefs.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("serverprefs: upsert failed: %w", err)
	}

	if prefs.ID == 0 {
		id, err := result.LastInsertId()
		if err == nil {
			prefs.ID = id
		}
	}
	return nil
}

// Close releases database resources.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}
