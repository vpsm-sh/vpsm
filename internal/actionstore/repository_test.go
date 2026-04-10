package actionstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "vpsm.db")
	r, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt failed: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func TestSave_Insert(t *testing.T) {
	r := tempRepo(t)

	record := &ActionRecord{
		ActionID:     "act-1",
		Provider:     "hetzner",
		ServerID:     "42",
		ServerName:   "web-1",
		Command:      "start_server",
		TargetStatus: "running",
		Status:       "running",
	}

	if err := r.Save(record); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if record.ID == 0 {
		t.Error("expected ID to be assigned after insert")
	}
	if record.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if record.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestSave_Update(t *testing.T) {
	r := tempRepo(t)

	record := &ActionRecord{
		ActionID:     "act-1",
		Provider:     "hetzner",
		ServerID:     "42",
		Command:      "start_server",
		TargetStatus: "running",
		Status:       "running",
	}

	if err := r.Save(record); err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	record.Status = "success"
	record.Progress = 100
	if err := r.Save(record); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	got, err := r.Get(record.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Status != "success" {
		t.Errorf("expected status 'success', got %q", got.Status)
	}
	if got.Progress != 100 {
		t.Errorf("expected progress 100, got %d", got.Progress)
	}
}

func TestSave_UpdateNotFound(t *testing.T) {
	r := tempRepo(t)

	record := &ActionRecord{ID: 999, Status: "running"}
	err := r.Save(record)
	if err == nil {
		t.Fatal("expected error updating non-existent record")
	}
}

func TestGet_NotFound(t *testing.T) {
	r := tempRepo(t)

	got, err := r.Get(999)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent record, got %+v", got)
	}
}

func TestGet_Found(t *testing.T) {
	r := tempRepo(t)

	record := &ActionRecord{
		ActionID: "act-1",
		Provider: "hetzner",
		ServerID: "42",
		Status:   "running",
	}
	r.Save(record)

	got, err := r.Get(record.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.ActionID != "act-1" {
		t.Errorf("expected ActionID 'act-1', got %q", got.ActionID)
	}
}

func TestListPending(t *testing.T) {
	r := tempRepo(t)

	// Insert mix of running and completed actions.
	for _, status := range []string{"running", "success", "running", "error"} {
		record := &ActionRecord{
			Provider: "hetzner",
			ServerID: "42",
			Status:   status,
		}
		r.Save(record)
	}

	pending, err := r.ListPending()
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending actions, got %d", len(pending))
	}
	for _, record := range pending {
		if record.Status != "running" {
			t.Errorf("expected status 'running', got %q", record.Status)
		}
	}
}

func TestListRecent(t *testing.T) {
	r := tempRepo(t)

	for i := range 5 {
		record := &ActionRecord{
			Provider:  "hetzner",
			ServerID:  "42",
			Status:    "success",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		r.Save(record)
	}

	recent, err := r.ListRecent(3)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent actions, got %d", len(recent))
	}
	// Should be sorted newest first.
	for i := 1; i < len(recent); i++ {
		if recent[i].CreatedAt.After(recent[i-1].CreatedAt) {
			t.Error("expected records sorted by created_at descending")
		}
	}
}

func TestListRecent_All(t *testing.T) {
	r := tempRepo(t)

	for range 3 {
		r.Save(&ActionRecord{Provider: "hetzner", ServerID: "42", Status: "success"})
	}

	// Request more than available.
	recent, err := r.ListRecent(10)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recent))
	}
}

func TestDeleteOlderThan(t *testing.T) {
	r := tempRepo(t)

	recent := &ActionRecord{
		Provider: "hetzner",
		ServerID: "43",
		Status:   "running",
	}
	r.Save(recent)

	completed := &ActionRecord{
		Provider: "hetzner",
		ServerID: "44",
		Status:   "success",
	}
	r.Save(completed)

	// Nothing should be deleted since everything is recent.
	removed, err := r.DeleteOlderThan(24 * time.Hour)
	if err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}

	// Delete everything older than 0 (all completed).
	removed, err = r.DeleteOlderThan(0)
	if err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Running action should still be there.
	pending, _ := r.ListPending()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending action remaining, got %d", len(pending))
	}
}

func TestSQLiteRepository_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpsm.db")

	// Write with one repository instance.
	r1, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt failed: %v", err)
	}
	record := &ActionRecord{
		ActionID: "act-1",
		Provider: "hetzner",
		ServerID: "42",
		Status:   "running",
	}
	if err := r1.Save(record); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	r1.Close()

	// Read with a new repository instance.
	r2, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt failed: %v", err)
	}
	defer r2.Close()

	got, err := r2.Get(record.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected record to be persisted, got nil")
	}
	if got.ActionID != "act-1" {
		t.Errorf("expected ActionID 'act-1', got %q", got.ActionID)
	}
}

func TestSQLiteRepository_EmptyDB(t *testing.T) {
	r := tempRepo(t)

	pending, err := r.ListPending()
	if err != nil {
		t.Fatalf("ListPending on empty repo failed: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending on empty repo, got %d", len(pending))
	}
}

func TestSQLiteRepository_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "vpsm.db")
	r, err := OpenAt(path)
	if err != nil {
		t.Fatalf("OpenAt failed to create nested directory: %v", err)
	}
	defer r.Close()

	record := &ActionRecord{Provider: "hetzner", ServerID: "42", Status: "running"}
	if err := r.Save(record); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist at %s, got error: %v", path, err)
	}
}
