package auditlog

import (
	"database/sql"
	"fmt"
	"time"

	"nathanbeddoewebdev/vpsm/internal/database"
)

// Repository defines the persistence interface for audit entries.
type Repository interface {
	Save(entry *AuditEntry) error
	List(limit int) ([]AuditEntry, error)
	ListByCommand(command string, limit int) ([]AuditEntry, error)
	Prune(olderThan time.Duration) (int64, error)
	Close() error
}

// SQLiteRepository implements Repository backed by a local SQLite database.
type SQLiteRepository struct {
	db *sql.DB
}

// Open creates or opens the audit repository at the default path.
func Open() (*SQLiteRepository, error) {
	path, err := database.DefaultPath()
	if err != nil {
		return nil, fmt.Errorf("auditlog: %w", err)
	}
	return OpenAt(path)
}

// OpenAt creates or opens a SQLite database at the given path.
func OpenAt(path string) (*SQLiteRepository, error) {
	db, err := database.Open(path)
	if err != nil {
		return nil, fmt.Errorf("auditlog: %w", err)
	}

	r := &SQLiteRepository{db: db}
	if err := r.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}

func (r *SQLiteRepository) migrate() error {
	const ddl = `
        CREATE TABLE IF NOT EXISTS audit_log (
            id            INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp     TEXT    NOT NULL,
            command       TEXT    NOT NULL,
            args          TEXT    NOT NULL DEFAULT '',
            provider      TEXT    NOT NULL DEFAULT '',
            resource_type TEXT    NOT NULL DEFAULT '',
            resource_id   TEXT    NOT NULL DEFAULT '',
            resource_name TEXT    NOT NULL DEFAULT '',
            outcome       TEXT    NOT NULL DEFAULT '',
            detail        TEXT    NOT NULL DEFAULT '',
            duration_ms   INTEGER NOT NULL DEFAULT 0
        );
        CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
        CREATE INDEX IF NOT EXISTS idx_audit_log_command ON audit_log(command);
        CREATE INDEX IF NOT EXISTS idx_audit_log_provider ON audit_log(provider);
        CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log(resource_type, resource_id);
    `
	if _, err := r.db.Exec(ddl); err != nil {
		return fmt.Errorf("auditlog: migration failed: %w", err)
	}
	return nil
}

// Save inserts a new audit entry.
func (r *SQLiteRepository) Save(entry *AuditEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	result, err := r.db.Exec(`
        INSERT INTO audit_log (timestamp, command, args, provider, resource_type, resource_id, resource_name, outcome, detail, duration_ms)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp.Format(time.RFC3339Nano), entry.Command, entry.Args, entry.Provider,
		entry.ResourceType, entry.ResourceID, entry.ResourceName, entry.Outcome, entry.Detail, entry.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("auditlog: insert failed: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("auditlog: failed to get last insert ID: %w", err)
	}
	entry.ID = id
	return nil
}

// List returns the most recent n audit entries.
func (r *SQLiteRepository) List(limit int) ([]AuditEntry, error) {
	rows, err := r.db.Query(`
        SELECT id, timestamp, command, args, provider, resource_type, resource_id, resource_name,
               outcome, detail, duration_ms
        FROM audit_log ORDER BY timestamp DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("auditlog: query failed: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// ListByCommand returns the most recent n audit entries for a command.
func (r *SQLiteRepository) ListByCommand(command string, limit int) ([]AuditEntry, error) {
	rows, err := r.db.Query(`
        SELECT id, timestamp, command, args, provider, resource_type, resource_id, resource_name,
               outcome, detail, duration_ms
        FROM audit_log WHERE command = ? ORDER BY timestamp DESC LIMIT ?`, command, limit)
	if err != nil {
		return nil, fmt.Errorf("auditlog: query failed: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// Prune deletes entries older than the given duration.
func (r *SQLiteRepository) Prune(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
	result, err := r.db.Exec(`DELETE FROM audit_log WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("auditlog: delete failed: %w", err)
	}
	return result.RowsAffected()
}

// Close releases database resources.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func scanRows(rows *sql.Rows) ([]AuditEntry, error) {
	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var timestampStr string
		err := rows.Scan(
			&entry.ID, &timestampStr, &entry.Command, &entry.Args, &entry.Provider,
			&entry.ResourceType, &entry.ResourceID, &entry.ResourceName,
			&entry.Outcome, &entry.Detail, &entry.DurationMs,
		)
		if err != nil {
			return nil, fmt.Errorf("auditlog: scan failed: %w", err)
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339Nano, timestampStr)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
