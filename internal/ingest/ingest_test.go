package ingest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFindMarkdownFiles(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create test files
	files := []string{
		"file1.md",
		"file2.md",
		"file3.txt",
		"subdir/file4.md",
		"subdir/file5.txt",
	}

	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("test content"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Test directory
	stat, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("failed to stat dir: %v", err)
	}

	mdFiles := findMarkdownFiles(tmpDir, stat)
	expectedCount := 3 // file1.md, file2.md, subdir/file4.md
	if len(mdFiles) != expectedCount {
		t.Errorf("expected %d markdown files, got %d: %v", expectedCount, len(mdFiles), mdFiles)
	}

	// Verify all returned files are .md
	for _, f := range mdFiles {
		if !strings.HasSuffix(strings.ToLower(f), ".md") {
			t.Errorf("expected .md file, got %s", f)
		}
	}

	// Test single file
	singleFile := filepath.Join(tmpDir, "file1.md")
	stat, err = os.Stat(singleFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	mdFiles = findMarkdownFiles(singleFile, stat)
	if len(mdFiles) != 1 {
		t.Errorf("expected 1 markdown file for single file, got %d", len(mdFiles))
	}
	if mdFiles[0] != singleFile {
		t.Errorf("expected %s, got %s", singleFile, mdFiles[0])
	}

	// Test non-markdown file
	txtFile := filepath.Join(tmpDir, "file3.txt")
	stat, err = os.Stat(txtFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	mdFiles = findMarkdownFiles(txtFile, stat)
	if len(mdFiles) != 0 {
		t.Errorf("expected 0 markdown files for .txt file, got %d", len(mdFiles))
	}
}

func TestDiffIds(t *testing.T) {
	tests := []struct {
		name     string
		oldIDs   []string
		newIDs   []string
		expected []string
	}{
		{
			name:     "both empty",
			oldIDs:   []string{},
			newIDs:   []string{},
			expected: []string{},
		},
		{
			name:     "old empty, new has items",
			oldIDs:   []string{},
			newIDs:   []string{"id1", "id2"},
			expected: []string{},
		},
		{
			name:     "old has items, new empty",
			oldIDs:   []string{"id1", "id2", "id3"},
			newIDs:   []string{},
			expected: []string{"id1", "id2", "id3"},
		},
		{
			name:     "no overlap",
			oldIDs:   []string{"id1", "id2", "id3"},
			newIDs:   []string{"id4", "id5"},
			expected: []string{"id1", "id2", "id3"},
		},
		{
			name:     "complete overlap",
			oldIDs:   []string{"id1", "id2", "id3"},
			newIDs:   []string{"id1", "id2", "id3"},
			expected: []string{},
		},
		{
			name:     "partial overlap",
			oldIDs:   []string{"id1", "id2", "id3", "id4"},
			newIDs:   []string{"id2", "id4", "id5"},
			expected: []string{"id1", "id3"},
		},
		{
			name:     "new has more items",
			oldIDs:   []string{"id1", "id2"},
			newIDs:   []string{"id1", "id2", "id3", "id4"},
			expected: []string{},
		},
		{
			name:     "single item overlap",
			oldIDs:   []string{"id1", "id2", "id3"},
			newIDs:   []string{"id2"},
			expected: []string{"id1", "id3"},
		},
		{
			name:     "duplicates in old",
			oldIDs:   []string{"id1", "id1", "id2"},
			newIDs:   []string{"id2"},
			expected: []string{"id1", "id1"},
		},
		{
			name:     "duplicates in new",
			oldIDs:   []string{"id1", "id2", "id3"},
			newIDs:   []string{"id1", "id1", "id2"},
			expected: []string{"id3"},
		},
		{
			name:     "unordered IDs",
			oldIDs:   []string{"id3", "id1", "id2"},
			newIDs:   []string{"id2", "id4", "id1"},
			expected: []string{"id3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := diffIds(tt.oldIDs, tt.newIDs)
			// Normalize nil and empty slices for comparison
			if len(result) == 0 && len(tt.expected) == 0 {
				// Both are empty, consider equal
				return
			}
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("diffIds(%v, %v) = %v, want %v", tt.oldIDs, tt.newIDs, result, tt.expected)
			}
		})
	}
}

func TestPointIDFromChunkKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "simple key",
			key:  "path/to/file.md\nSection/Subsection\n0\nabc123",
		},
		{
			name: "key with special characters",
			key:  "notes/test.md\nIntroduction\n1\nhash456",
		},
		{
			name: "empty heading path",
			key:  "file.md\n\n0\nhash789",
		},
		{
			name: "multi-level heading",
			key:  "doc.md\nChapter 1/Section A/Subsection\n2\nhash012",
		},
	}

	// Test 1: Same key produces same UUID (deterministic)
	for _, tt := range tests {
		t.Run(tt.name+"_deterministic", func(t *testing.T) {
			id1 := PointIDFromChunkKey(tt.key)
			id2 := PointIDFromChunkKey(tt.key)
			id3 := PointIDFromChunkKey(tt.key)

			if id1 != id2 || id2 != id3 {
				t.Errorf("PointIDFromChunkKey not deterministic: got %q, %q, %q for same key", id1, id2, id3)
			}

			// Verify it's a valid UUID format (should be 36 chars with dashes)
			if len(id1) != 36 {
				t.Errorf("PointIDFromChunkKey returned invalid UUID length: got %d, want 36", len(id1))
			}
		})
	}

	// Test 2: Different keys produce different UUIDs
	t.Run("different_keys_produce_different_ids", func(t *testing.T) {
		keys := []string{
			"file1.md\nSection\n0\nhash1",
			"file2.md\nSection\n0\nhash1",
			"file1.md\nDifferentSection\n0\nhash1",
			"file1.md\nSection\n1\nhash1",
			"file1.md\nSection\n0\nhash2",
		}

		ids := make(map[string]string)
		for _, key := range keys {
			id := PointIDFromChunkKey(key)
			if existingKey, exists := ids[id]; exists {
				t.Errorf("Collision detected: keys %q and %q both produce ID %q", existingKey, key, id)
			}
			ids[id] = key
		}

		// Verify we got different IDs for all different keys
		if len(ids) != len(keys) {
			t.Errorf("Expected %d unique IDs, got %d", len(keys), len(ids))
		}
	})

	// Test 3: Minimal changes in key produce different UUIDs
	t.Run("minimal_key_changes", func(t *testing.T) {
		baseKey := "notes/test.md\nIntroduction\n0\nabc123"
		baseID := PointIDFromChunkKey(baseKey)

		variations := []struct {
			name string
			key  string
		}{
			{"change source path", "notes/test2.md\nIntroduction\n0\nabc123"},
			{"change heading", "notes/test.md\nConclusion\n0\nabc123"},
			{"change chunk index", "notes/test.md\nIntroduction\n1\nabc123"},
			{"change content hash", "notes/test.md\nIntroduction\n0\nxyz789"},
		}

		for _, v := range variations {
			id := PointIDFromChunkKey(v.key)
			if id == baseID {
				t.Errorf("%s: expected different ID, got same ID %q", v.name, id)
			}
		}
	})

	// Test 4: Integration test - chunk key construction matches processChunks usage
	t.Run("integration_chunk_key_construction", func(t *testing.T) {
		sourcePath := "notes/test.md"
		headingPath := []string{"Introduction", "Overview"}
		contentHash := "abc123def456"

		chunkKey := sourcePath + "\n" + strings.Join(headingPath, "/") + "\n" + "2" + "\n" + contentHash
		pointID := PointIDFromChunkKey(chunkKey)

		// Verify it's a valid UUID
		if len(pointID) != 36 {
			t.Errorf("expected UUID length 36, got %d", len(pointID))
		}

		// Verify deterministic
		pointID2 := PointIDFromChunkKey(chunkKey)
		if pointID != pointID2 {
			t.Errorf("PointIDFromChunkKey not deterministic: got %q and %q", pointID, pointID2)
		}

		// Verify different keys produce different IDs
		chunkKey2 := sourcePath + "\n" + strings.Join(headingPath, "/") + "\n" + "3" + "\n" + contentHash
		pointID3 := PointIDFromChunkKey(chunkKey2)
		if pointID == pointID3 {
			t.Errorf("different chunk keys produced same point ID")
		}
	})
}

func TestDiffIds_IngestUseCase(t *testing.T) {
	// Test the specific use case: finding what to delete and what to upsert
	oldIDs := []string{"id1", "id2", "id3", "id4"}
	newIDs := []string{"id2", "id4", "id5", "id6"}

	// What to delete: old - new
	deleteIDs := diffIds(oldIDs, newIDs)
	expectedDelete := []string{"id1", "id3"}
	if !equalStringSlices(deleteIDs, expectedDelete) {
		t.Errorf("deleteIDs = %v, want %v", deleteIDs, expectedDelete)
	}

	// What to upsert: new - old
	upsertIDs := diffIds(newIDs, oldIDs)
	expectedUpsert := []string{"id5", "id6"}
	if !equalStringSlices(upsertIDs, expectedUpsert) {
		t.Errorf("upsertIDs = %v, want %v", upsertIDs, expectedUpsert)
	}

	// Verify intersection (id2, id4) is not in either result
	intersection := []string{"id2", "id4"}
	for _, id := range intersection {
		if contains(deleteIDs, id) {
			t.Errorf("deleteIDs should not contain intersection ID %s", id)
		}
		if contains(upsertIDs, id) {
			t.Errorf("upsertIDs should not contain intersection ID %s", id)
		}
	}
}

// Helper functions for testing
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSet := make(map[string]bool)
	for _, x := range a {
		aSet[x] = true
	}
	for _, x := range b {
		if !aSet[x] {
			return false
		}
	}
	return true
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
