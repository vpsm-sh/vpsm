package auditlog

import "time"

const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// AuditEntry represents a persisted audit event.
type AuditEntry struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Command      string    `json:"command"`
	Args         string    `json:"args,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	ResourceName string    `json:"resource_name,omitempty"`
	Outcome      string    `json:"outcome"`
	Detail       string    `json:"detail,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
}
