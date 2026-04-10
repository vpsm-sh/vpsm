package database

import (
	"path/filepath"
	"testing"
)

func TestDefaultPathOverride(t *testing.T) {
	t.Cleanup(ResetPath)

	path := filepath.Join(t.TempDir(), "vpsm.db")
	SetPath(path)

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath error: %v", err)
	}
	if got != path {
		t.Fatalf("DefaultPath = %q, want %q", got, path)
	}
}

func TestOpenCreatesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vpsm.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
}
