package tui

import (
	"time"

	"nathanbeddoewebdev/vpsm/internal/auditlog"
)

// recordAudit writes a best-effort audit entry for a TUI-initiated operation.
// Errors opening the repository or saving the entry are silently discarded —
// the same policy used by the centralized writer in cmd/root.go.
func recordAudit(providerName, command, resourceType, resourceID, resourceName string, err error, start time.Time) {
	repo, openErr := auditlog.Open()
	if openErr != nil {
		return
	}
	defer repo.Close()

	entry := &auditlog.AuditEntry{
		Timestamp:    start,
		Command:      command,
		Provider:     providerName,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		DurationMs:   time.Since(start).Milliseconds(),
	}
	if err != nil {
		entry.Outcome = auditlog.OutcomeError
		entry.Detail = err.Error()
	} else {
		entry.Outcome = auditlog.OutcomeSuccess
	}
	_ = repo.Save(entry)
}
