// Package actionstore provides persistent storage for in-flight provider actions.
//
// When a user starts or stops a server, the CLI tracks the action locally
// so that if the process is interrupted (Ctrl+C, crash, etc.) the action
// can be resumed on the next invocation.
//
// Storage is backed by a SQLite database at ~/.config/vpsm/vpsm.db
// (or the platform-equivalent path returned by os.UserConfigDir).
package actionstore

import (
	"database/sql"
	"fmt"
	"time"

	"nathanbeddoewebdev/vpsm/internal/database"
)

// SetPath overrides the database path. Intended for testing.
func SetPath(p string) { database.SetPath(p) }

// ResetPath clears the path override. Intended for testing.
func ResetPath() { database.ResetPath() }

// ActionRepository defines the persistence interface for action records.
type ActionRepository interface {
	// Save inserts or updates an action record. On insert (ID == 0), an
	// ID is assigned to the record.
	Save(record *ActionRecord) error

	// Get retrieves a single action record by ID.
	Get(id int64) (*ActionRecord, error)

	// ListPending returns all action records with status "running",
	// ordered by creation time (newest first).
	ListPending() ([]ActionRecord, error)

	// ListRecent returns the most recent n action records regardless of
	// status, ordered by creation time (newest first).
	ListRecent(n int) ([]ActionRecord, error)

	// DeleteOlderThan removes completed/errored records older than d.
	// Returns the number of records removed.
	DeleteOlderThan(d time.Duration) (int64, error)

	// Close releases database resources.
	Close() error
}

// SQLiteRepository implements ActionRepository backed by a local SQLite database.
type SQLiteRepository struct {
	db *sql.DB
}

// DefaultPath returns the default database path.
func DefaultPath() (string, error) {
	path, err := database.DefaultPath()
	if err != nil {
		return "", fmt.Errorf("actions: %w", err)
	}
	return path, nil
}

// Open creates or opens the action repository at the default path.
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
		return nil, fmt.Errorf("actions: %w", err)
	}

	r := &SQLiteRepository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return r, nil
}

// migrate creates the actions table if it doesn't exist.
func (r *SQLiteRepository) migrate() error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS actions (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			action_id     TEXT    NOT NULL DEFAULT '',
			provider      TEXT    NOT NULL,
			server_id     TEXT    NOT NULL,
			server_name   TEXT    NOT NULL DEFAULT '',
			command       TEXT    NOT NULL DEFAULT '',
			target_status TEXT    NOT NULL DEFAULT '',
			status        TEXT    NOT NULL DEFAULT 'running',
			progress      INTEGER NOT NULL DEFAULT 0,
			error_message TEXT    NOT NULL DEFAULT '',
			created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
			updated_at    TEXT    NOT NULL DEFAULT (datetime('now'))
		);
		CREATE INDEX IF NOT EXISTS idx_actions_status ON actions(status);
	`
	if _, err := r.db.Exec(ddl); err != nil {
		return fmt.Errorf("actions: migration failed: %w", err)
	}
	return nil
}

// Save inserts a new record (ID == 0) or updates an existing one.
func (r *SQLiteRepository) Save(record *ActionRecord) error {
	record.UpdatedAt = time.Now().UTC()

	if record.ID == 0 {
		// Insert
		if record.CreatedAt.IsZero() {
			record.CreatedAt = record.UpdatedAt
		}
		result, err := r.db.Exec(`
			INSERT INTO actions (action_id, provider, server_id, server_name, command, target_status, status, progress, error_message, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.ActionID, record.Provider, record.ServerID, record.ServerName, record.Command,
			record.TargetStatus, record.Status, record.Progress, record.ErrorMessage,
			record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano),
		)
		if err != nil {
			return fmt.Errorf("actions: insert failed: %w", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("actions: failed to get last insert ID: %w", err)
		}
		record.ID = id
		return nil
	}

	// Update
	result, err := r.db.Exec(`
		UPDATE actions SET action_id=?, provider=?, server_id=?, server_name=?,
		       command=?, target_status=?, status=?, progress=?, error_message=?,
		       updated_at=?
		WHERE id=?`,
		record.ActionID, record.Provider, record.ServerID, record.ServerName, record.Command,
		record.TargetStatus, record.Status, record.Progress, record.ErrorMessage,
		record.UpdatedAt.Format(time.RFC3339Nano), record.ID,
	)
	if err != nil {
		return fmt.Errorf("actions: update failed: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("actions: action with ID %d not found", record.ID)
	}
	return nil
}

// Get retrieves a single action record by ID.
func (r *SQLiteRepository) Get(id int64) (*ActionRecord, error) {
	row := r.db.QueryRow(`
		SELECT id, action_id, provider, server_id, server_name, command,
		       target_status, status, progress, error_message, created_at, updated_at
		FROM actions WHERE id = ?`, id)

	record, err := scanRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("actions: query failed: %w", err)
	}
	return record, nil
}

// ListPending returns all action records with status "running".
func (r *SQLiteRepository) ListPending() ([]ActionRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, action_id, provider, server_id, server_name, command,
		       target_status, status, progress, error_message, created_at, updated_at
		FROM actions WHERE status = 'running' ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("actions: query failed: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// ListRecent returns the most recent n action records regardless of status.
func (r *SQLiteRepository) ListRecent(n int) ([]ActionRecord, error) {
	rows, err := r.db.Query(`
		SELECT id, action_id, provider, server_id, server_name, command,
		       target_status, status, progress, error_message, created_at, updated_at
		FROM actions ORDER BY created_at DESC LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("actions: query failed: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// DeleteOlderThan removes completed/errored records older than d.
func (r *SQLiteRepository) DeleteOlderThan(d time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-d).Format(time.RFC3339Nano)
	result, err := r.db.Exec(`
		DELETE FROM actions WHERE status != 'running' AND updated_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("actions: delete failed: %w", err)
	}
	return result.RowsAffected()
}

// Close releases database resources.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// scanRow scans a single row into an ActionRecord.
func scanRow(row *sql.Row) (*ActionRecord, error) {
	var record ActionRecord
	var createdStr, updatedStr string
	err := row.Scan(
		&record.ID, &record.ActionID, &record.Provider, &record.ServerID, &record.ServerName,
		&record.Command, &record.TargetStatus, &record.Status, &record.Progress, &record.ErrorMessage,
		&createdStr, &updatedStr,
	)
	if err != nil {
		return nil, err
	}
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return &record, nil
}

// scanRows scans multiple rows into ActionRecords.
func scanRows(rows *sql.Rows) ([]ActionRecord, error) {
	var records []ActionRecord
	for rows.Next() {
		var record ActionRecord
		var createdStr, updatedStr string
		err := rows.Scan(
			&record.ID, &record.ActionID, &record.Provider, &record.ServerID, &record.ServerName,
			&record.Command, &record.TargetStatus, &record.Status, &record.Progress, &record.ErrorMessage,
			&createdStr, &updatedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("actions: scan failed: %w", err)
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
		records = append(records, record)
	}
	return records, rows.Err()
}
