package auditlog

import (
	"path/filepath"
	"testing"
	"time"
)

func tempRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vpsm.db")
	r, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt failed: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestSave_AssignsIDAndTimestamp(t *testing.T) {
	r := tempRepo(t)

	entry := &AuditEntry{
		Command:    "vpsm server list",
		Outcome:    OutcomeSuccess,
		DurationMs: 12,
	}

	if err := r.Save(entry); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected ID to be assigned")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected Timestamp to be set")
	}
}

func TestList(t *testing.T) {
	r := tempRepo(t)

	for i := range 3 {
		entry := &AuditEntry{
			Command:   "vpsm server list",
			Outcome:   OutcomeSuccess,
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		if err := r.Save(entry); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	entries, err := r.List(2)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Timestamp.Before(entries[1].Timestamp) {
		t.Error("expected entries sorted by timestamp descending")
	}
}

func TestListByCommand(t *testing.T) {
	r := tempRepo(t)

	entries := []*AuditEntry{
		{Command: "vpsm server list", Outcome: OutcomeSuccess},
		{Command: "vpsm server create", Outcome: OutcomeSuccess},
		{Command: "vpsm server list", Outcome: OutcomeError},
	}
	for _, entry := range entries {
		if err := r.Save(entry); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	listEntries, err := r.ListByCommand("vpsm server list", 10)
	if err != nil {
		t.Fatalf("ListByCommand failed: %v", err)
	}
	if len(listEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(listEntries))
	}
	for _, entry := range listEntries {
		if entry.Command != "vpsm server list" {
			t.Errorf("expected command 'vpsm server list', got %q", entry.Command)
		}
	}
}

func TestPrune(t *testing.T) {
	r := tempRepo(t)

	oldEntry := &AuditEntry{
		Command:   "vpsm server list",
		Outcome:   OutcomeSuccess,
		Timestamp: time.Now().UTC().Add(-48 * time.Hour),
	}
	recentEntry := &AuditEntry{
		Command:   "vpsm server list",
		Outcome:   OutcomeSuccess,
		Timestamp: time.Now().UTC().Add(-1 * time.Hour),
	}

	if err := r.Save(oldEntry); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := r.Save(recentEntry); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	removed, err := r.Prune(24 * time.Hour)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	remaining, err := r.List(10)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining entry, got %d", len(remaining))
	}
}
