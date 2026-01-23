package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad_MissingFile(t *testing.T) {
	// Load from non-existent file should return empty manifest
	tmpDir := t.TempDir()
	missingPath := filepath.Join(tmpDir, "nonexistent.json")

	m, err := Load(missingPath)
	if err != nil {
		t.Fatalf("Load should not error on missing file, got: %v", err)
	}
	if m == nil {
		t.Fatal("Load should return non-nil manifest")
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if m.Files == nil {
		t.Fatal("Files map should be initialized")
	}
	if len(m.Files) != 0 {
		t.Errorf("Files map should be empty, got %d entries", len(m.Files))
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "test.json")

	// Create manifest with some data
	m := &Manifest{
		Version: 1,
		Files: map[string]FileEntry{
			"test.md": {
				Mtime:    1234567890,
				PointIDs: []string{"id1", "id2"},
			},
		},
	}

	// Save should create temp file first, then rename
	if err := Save(manifestPath, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest file should exist: %v", err)
	}

	// Verify temp file doesn't exist
	tmpPath := manifestPath + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temp file should not exist after save")
	}

	// Verify content
	loaded, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("failed to load saved manifest: %v", err)
	}
	if loaded.Version != m.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, m.Version)
	}
	if len(loaded.Files) != 1 {
		t.Errorf("Files count = %d, want 1", len(loaded.Files))
	}
	entry, ok := loaded.Files["test.md"]
	if !ok {
		t.Fatal("test.md entry not found")
	}
	if entry.Mtime != 1234567890 {
		t.Errorf("Mtime = %d, want 1234567890", entry.Mtime)
	}
	if !reflect.DeepEqual(entry.PointIDs, []string{"id1", "id2"}) {
		t.Errorf("PointIDs = %v, want [id1 id2]", entry.PointIDs)
	}
}

func TestGetFile_SetFile(t *testing.T) {
	m := &Manifest{
		Version: 1,
		Files:   make(map[string]FileEntry),
	}

	// GetFile on empty manifest should return false
	_, ok := m.GetFile("nonexistent.md")
	if ok {
		t.Error("GetFile should return false for nonexistent file")
	}

	// SetFile and then GetFile
	entry := FileEntry{
		Mtime:    1234567890,
		PointIDs: []string{"id1", "id2", "id3"},
	}
	m.SetFile("test.md", entry)

	retrieved, ok := m.GetFile("test.md")
	if !ok {
		t.Fatal("GetFile should return true for existing file")
	}
	if retrieved.Mtime != entry.Mtime {
		t.Errorf("Mtime = %d, want %d", retrieved.Mtime, entry.Mtime)
	}
	if !reflect.DeepEqual(retrieved.PointIDs, entry.PointIDs) {
		t.Errorf("PointIDs = %v, want %v", retrieved.PointIDs, entry.PointIDs)
	}
}

func TestDiffOldNew_Deletes(t *testing.T) {
	// Test the diff logic used in ingest: old - new = deletes
	oldIDs := []string{"id1", "id2", "id3", "id4"}
	newIDs := []string{"id2", "id4", "id5"}

	// What should be deleted: old - new = ["id1", "id3"]
	expectedDeletes := []string{"id1", "id3"}

	// Simulate the diff logic from ingest package
	newIDsSet := make(map[string]struct{})
	for _, id := range newIDs {
		newIDsSet[id] = struct{}{}
	}

	var deletes []string
	for _, id := range oldIDs {
		if _, ok := newIDsSet[id]; !ok {
			deletes = append(deletes, id)
		}
	}

	if len(deletes) != len(expectedDeletes) {
		t.Fatalf("deletes length = %d, want %d", len(deletes), len(expectedDeletes))
	}

	deletesSet := make(map[string]bool)
	for _, id := range deletes {
		deletesSet[id] = true
	}
	for _, expected := range expectedDeletes {
		if !deletesSet[expected] {
			t.Errorf("expected delete ID %q not found in deletes", expected)
		}
	}

	// Verify upserts: new - old = ["id5"]
	var upserts []string
	oldIDsSet := make(map[string]struct{})
	for _, id := range oldIDs {
		oldIDsSet[id] = struct{}{}
	}
	for _, id := range newIDs {
		if _, ok := oldIDsSet[id]; !ok {
			upserts = append(upserts, id)
		}
	}

	if len(upserts) != 1 || upserts[0] != "id5" {
		t.Errorf("upserts = %v, want [id5]", upserts)
	}
}

func TestSave_NilManifest(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "test.json")

	err := Save(manifestPath, nil)
	if err == nil {
		t.Error("Save should error on nil manifest")
	}
}
