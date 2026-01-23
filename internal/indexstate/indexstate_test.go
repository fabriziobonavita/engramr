package indexstate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFileStore_Load_MissingFile(t *testing.T) {
	// Load from non-existent file should return empty index state
	tmpDir := t.TempDir()
	store := NewFileStore()

	state, err := store.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load should not error on missing file, got: %v", err)
	}
	if state == nil {
		t.Fatal("Load should return non-nil index state")
	}
	if state.Version != 1 {
		t.Errorf("Version = %d, want 1", state.Version)
	}
	if state.Files == nil {
		t.Fatal("Files map should be initialized")
	}
	if len(state.Files) != 0 {
		t.Errorf("Files map should be empty, got %d entries", len(state.Files))
	}
}

func TestFileStore_Save_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	// Create index state with some data
	state := &IndexState{
		Version: 1,
		Files: map[string]FileEntry{
			"test.md": {
				Mtime:    1234567890,
				PointIDs: []string{"id1", "id2"},
			},
		},
	}

	// Save should create temp file first, then rename
	if err := store.Save(tmpDir, state); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	indexPath := filepath.Join(tmpDir, "index.json")
	// Verify file exists
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index state file should exist: %v", err)
	}

	// Verify temp file doesn't exist
	tmpPath := indexPath + ".tmp"
	if _, err := os.Stat(tmpPath); err == nil {
		t.Error("temp file should not exist after save")
	}

	// Verify content
	loaded, err := store.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load saved index state: %v", err)
	}
	if loaded.Version != state.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, state.Version)
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

func TestIndexState_GetFile_SetFile(t *testing.T) {
	state := &IndexState{
		Version: 1,
		Files:   make(map[string]FileEntry),
	}

	// GetFile on empty index state should return false
	_, ok := state.GetFile("nonexistent.md")
	if ok {
		t.Error("GetFile should return false for nonexistent file")
	}

	// SetFile and then GetFile
	entry := FileEntry{
		Mtime:    1234567890,
		PointIDs: []string{"id1", "id2", "id3"},
	}
	state.SetFile("test.md", entry)

	retrieved, ok := state.GetFile("test.md")
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

func TestFileStore_Save_NilIndexState(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	err := store.Save(tmpDir, nil)
	if err == nil {
		t.Error("Save should error on nil index state")
	}
}

func TestFileStore_Transaction(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	// Test transaction with read-modify-write
	err := store.Transaction(tmpDir, func(state *IndexState) error {
		// Modify state
		state.SetFile("test.md", FileEntry{
			Mtime:    1234567890,
			PointIDs: []string{"id1", "id2"},
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Verify state was saved
	loaded, err := store.Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load index state: %v", err)
	}
	entry, ok := loaded.GetFile("test.md")
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

func TestFileStore_PutFile_GetFile_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	// Put file entry
	entry := FileEntry{
		Mtime:    1234567890,
		PointIDs: []string{"id1", "id2", "id3"},
	}

	err := store.Transaction(tmpDir, func(state *IndexState) error {
		state.SetFile("test.md", entry)
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction failed: %v", err)
	}

	// Get file entry
	loaded, err := store.Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	retrieved, ok := loaded.GetFile("test.md")
	if !ok {
		t.Fatal("test.md entry not found")
	}
	if retrieved.Mtime != entry.Mtime {
		t.Errorf("Mtime = %d, want %d", retrieved.Mtime, entry.Mtime)
	}
	if !reflect.DeepEqual(retrieved.PointIDs, entry.PointIDs) {
		t.Errorf("PointIDs = %v, want %v", retrieved.PointIDs, entry.PointIDs)
	}
}

// TestFileStore_FreshRepo tests that all operations work when the base directory doesn't exist yet.
// This simulates a fresh repo checkout where .engramr/ doesn't exist.
func TestFileStore_FreshRepo(t *testing.T) {
	// Create a temp dir, but use a subdirectory that doesn't exist yet
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, ".engramr")
	store := NewFileStore()

	// Verify the directory doesn't exist
	if _, err := os.Stat(baseDir); err == nil {
		t.Fatal("baseDir should not exist at start of test")
	}

	// Test Load: should work and create the directory
	state, err := store.Load(baseDir)
	if err != nil {
		t.Fatalf("Load should work on non-existent directory, got: %v", err)
	}
	if state == nil {
		t.Fatal("Load should return non-nil index state")
	}
	if state.Version != 1 {
		t.Errorf("Version = %d, want 1", state.Version)
	}
	// Verify directory was created
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatalf("Load should create baseDir, got error: %v", err)
	}

	// Test Save: should work on non-existent directory (though it exists now from Load)
	// Remove the directory to test Save creating it
	if err := os.RemoveAll(baseDir); err != nil {
		t.Fatalf("Failed to remove baseDir: %v", err)
	}

	testState := &IndexState{
		Version: 1,
		Files: map[string]FileEntry{
			"test.md": {
				Mtime:    1234567890,
				PointIDs: []string{"id1", "id2"},
			},
		},
	}
	if err := store.Save(baseDir, testState); err != nil {
		t.Fatalf("Save should work on non-existent directory, got: %v", err)
	}
	// Verify directory was created
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatalf("Save should create baseDir, got error: %v", err)
	}

	// Test Transaction: should work on non-existent directory
	// Remove the directory to test Transaction creating it
	if err := os.RemoveAll(baseDir); err != nil {
		t.Fatalf("Failed to remove baseDir: %v", err)
	}

	err = store.Transaction(baseDir, func(state *IndexState) error {
		state.SetFile("transaction.md", FileEntry{
			Mtime:    9876543210,
			PointIDs: []string{"id3", "id4"},
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction should work on non-existent directory, got: %v", err)
	}
	// Verify directory was created
	if _, err := os.Stat(baseDir); err != nil {
		t.Fatalf("Transaction should create baseDir, got error: %v", err)
	}

	// Verify the transaction actually saved data
	loaded, err := store.Load(baseDir)
	if err != nil {
		t.Fatalf("Failed to load after transaction: %v", err)
	}
	entry, ok := loaded.GetFile("transaction.md")
	if !ok {
		t.Fatal("transaction.md entry not found after Transaction")
	}
	if entry.Mtime != 9876543210 {
		t.Errorf("Mtime = %d, want 9876543210", entry.Mtime)
	}
	if !reflect.DeepEqual(entry.PointIDs, []string{"id3", "id4"}) {
		t.Errorf("PointIDs = %v, want [id3 id4]", entry.PointIDs)
	}
}
