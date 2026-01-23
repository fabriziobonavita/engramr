package ingest

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fabriziobonavita/engramr/internal/extract"
)

func TestIngestor_DeterministicIDs(t *testing.T) {
	// Test that the same chunk content produces the same point ID
	embedder := newFakeEmbedder()
	store := newFakeStore()
	manifestStore := newFakeManifestStore()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	ing := &Ingestor{
		Collection:   "test",
		ManifestPath: manifestPath,
		Embedder:     embedder,
		Store:        store,
		Manifest:     manifestStore,
	}

	ctx := context.Background()
	if err := ing.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create test markdown file
	testFile := filepath.Join(tmpDir, "test.md")
	content := "# Heading\n\nContent here with enough text to create a chunk that is meaningful."
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Ingest first time
	_, err := ing.IngestPath(ctx, testFile)
	if err != nil {
		t.Fatalf("IngestPath failed: %v", err)
	}

	// Get point IDs from manifest
	m, _ := manifestStore.Load(manifestPath)
	entry, ok := m.GetFile("test.md")
	if !ok {
		t.Fatal("test.md not found in manifest")
	}
	firstIDs := entry.PointIDs
	if len(firstIDs) == 0 {
		t.Fatal("no point IDs in manifest")
	}

	// Ingest again - should produce same IDs
	sum2, err := ing.IngestPath(ctx, testFile)
	if err != nil {
		t.Fatalf("IngestPath failed second time: %v", err)
	}

	m2, _ := manifestStore.Load(manifestPath)
	entry2, ok := m2.GetFile("test.md")
	if !ok {
		t.Fatal("test.md not found in manifest after second ingest")
	}
	secondIDs := entry2.PointIDs

	// IDs should be the same (deterministic)
	if !reflect.DeepEqual(firstIDs, secondIDs) {
		t.Errorf("point IDs not deterministic: first=%v, second=%v", firstIDs, secondIDs)
	}

	// Second ingest should skip chunks (no upserts, no deletes)
	if sum2.ChunksUpserted != 0 {
		t.Errorf("second ingest should skip chunks, got %d upserted", sum2.ChunksUpserted)
	}
	if sum2.ChunksDeleted != 0 {
		t.Errorf("second ingest should not delete, got %d deleted", sum2.ChunksDeleted)
	}
	if sum2.ChunksSkipped == 0 {
		t.Error("second ingest should have skipped chunks")
	}
}

func TestIngestor_DeletesStaleIDs(t *testing.T) {
	embedder := newFakeEmbedder()
	store := newFakeStore()
	manifestStore := newFakeManifestStore()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	ing := &Ingestor{
		Collection:   "test",
		ManifestPath: manifestPath,
		Embedder:     embedder,
		Store:        store,
		Manifest:     manifestStore,
	}

	ctx := context.Background()
	if err := ing.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")

	// First ingest with content that produces 2 chunks
	content1 := "# Heading 1\n\n" + strings.Repeat("a", 2000) + "\n\n## Heading 2\n\n" + strings.Repeat("b", 2000)
	if err := os.WriteFile(testFile, []byte(content1), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := ing.IngestPath(ctx, testFile)
	if err != nil {
		t.Fatalf("IngestPath failed: %v", err)
	}

	// Get old IDs
	m1, _ := manifestStore.Load(manifestPath)
	oldEntry, _ := m1.GetFile("test.md")
	oldIDs := oldEntry.PointIDs
	if len(oldIDs) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(oldIDs))
	}

	// Modify file to produce different chunks (remove one section)
	content2 := "# Heading 1\n\n" + strings.Repeat("a", 2000)
	if err := os.WriteFile(testFile, []byte(content2), 0644); err != nil {
		t.Fatalf("failed to write modified test file: %v", err)
	}

	// Second ingest
	sum2, err := ing.IngestPath(ctx, testFile)
	if err != nil {
		t.Fatalf("IngestPath failed second time: %v", err)
	}

	// Get new IDs
	m2, _ := manifestStore.Load(manifestPath)
	newEntry, _ := m2.GetFile("test.md")
	newIDs := newEntry.PointIDs

	// Verify deletes: old - new
	deleted := store.getDeletedPoints("test")
	if len(deleted) == 0 {
		t.Error("expected some points to be deleted")
	}

	// Verify deleted IDs are in old but not in new
	deletedSet := make(map[string]bool)
	for _, id := range deleted {
		deletedSet[id] = true
	}
	for _, id := range deleted {
		foundInOld := false
		for _, oldID := range oldIDs {
			if oldID == id {
				foundInOld = true
				break
			}
		}
		if !foundInOld {
			t.Errorf("deleted ID %q not found in old IDs", id)
		}
		foundInNew := false
		for _, newID := range newIDs {
			if newID == id {
				foundInNew = true
				break
			}
		}
		if foundInNew {
			t.Errorf("deleted ID %q found in new IDs", id)
		}
	}

	if sum2.ChunksDeleted == 0 {
		t.Error("expected chunks to be deleted")
	}
}

func TestIngestor_UpsertsNewPoints(t *testing.T) {
	embedder := newFakeEmbedder()
	store := newFakeStore()
	manifestStore := newFakeManifestStore()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	ing := &Ingestor{
		Collection:   "test",
		ManifestPath: manifestPath,
		Embedder:     embedder,
		Store:        store,
		Manifest:     manifestStore,
	}

	ctx := context.Background()
	if err := ing.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")
	content := "# Heading\n\nContent here with enough text to create a chunk that is meaningful and has sufficient length."
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Ingest
	sum, err := ing.IngestPath(ctx, testFile)
	if err != nil {
		t.Fatalf("IngestPath failed: %v", err)
	}

	// Verify points were upserted
	points := store.getPoints("test")
	if len(points) == 0 {
		t.Fatal("no points were upserted")
	}

	// Verify point structure
	for _, p := range points {
		if p.ID == "" {
			t.Error("point ID is empty")
		}
		if len(p.Vector) == 0 {
			t.Error("point vector is empty")
		}
		if p.Payload == nil {
			t.Error("point payload is nil")
		}
		if p.Payload["source_path"] != "test.md" {
			t.Errorf("source_path = %v, want test.md", p.Payload["source_path"])
		}
		if p.Payload["content"] == nil {
			t.Error("content is missing from payload")
		}
	}

	if sum.ChunksUpserted == 0 {
		t.Error("expected chunks to be upserted")
	}
	if sum.FilesIngested != 1 {
		t.Errorf("FilesIngested = %d, want 1", sum.FilesIngested)
	}
}

func TestIngestor_UpdatesManifestEntry(t *testing.T) {
	embedder := newFakeEmbedder()
	store := newFakeStore()
	manifestStore := newFakeManifestStore()

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.json")

	ing := &Ingestor{
		Collection:   "test",
		ManifestPath: manifestPath,
		Embedder:     embedder,
		Store:        store,
		Manifest:     manifestStore,
	}

	ctx := context.Background()
	if err := ing.Init(ctx); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "test.md")
	content := "# Heading\n\nContent here with enough text to create a chunk."
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Ingest
	_, err := ing.IngestPath(ctx, testFile)
	if err != nil {
		t.Fatalf("IngestPath failed: %v", err)
	}

	// Verify manifest was saved
	m := manifestStore.getManifest(manifestPath)
	if m == nil {
		t.Fatal("manifest was not saved")
	}

	// Verify entry exists
	entry, ok := m.GetFile("test.md")
	if !ok {
		t.Fatal("test.md entry not found in manifest")
	}

	// Verify entry has point IDs
	if len(entry.PointIDs) == 0 {
		t.Error("entry has no point IDs")
	}

	// Verify mtime is set
	if entry.Mtime == 0 {
		t.Error("entry mtime is not set")
	}
}

func TestPointIDFromChunkKey_Deterministic(t *testing.T) {
	chunkKey := "test.md\nHeading\n0\nabc123"
	id1 := PointIDFromChunkKey(chunkKey)
	id2 := PointIDFromChunkKey(chunkKey)
	id3 := PointIDFromChunkKey(chunkKey)

	if id1 != id2 || id2 != id3 {
		t.Errorf("PointIDFromChunkKey not deterministic: %q, %q, %q", id1, id2, id3)
	}

	// Different keys should produce different IDs
	chunkKey2 := "test.md\nHeading\n1\nabc123"
	id4 := PointIDFromChunkKey(chunkKey2)
	if id1 == id4 {
		t.Error("different chunk keys produced same ID")
	}
}

func TestProcessChunks_DeterministicIDs(t *testing.T) {
	embedder := newFakeEmbedder()
	ing := &Ingestor{
		Embedder: embedder,
	}

	chunks := []extract.Chunk{
		{
			SourcePath:  "test.md",
			HeadingPath: []string{"Heading"},
			ChunkIndex:  0,
			Content:     "Content here",
		},
	}

	ctx := context.Background()
	var sum IngestSummary

	// Process chunks twice
	ids1, points1, err := ing.processChunks(ctx, chunks, 1234567890, &sum)
	if err != nil {
		t.Fatalf("processChunks failed: %v", err)
	}

	var sum2 IngestSummary
	ids2, points2, err := ing.processChunks(ctx, chunks, 1234567890, &sum2)
	if err != nil {
		t.Fatalf("processChunks failed second time: %v", err)
	}

	// IDs should be deterministic
	if !reflect.DeepEqual(ids1, ids2) {
		t.Errorf("point IDs not deterministic: %v vs %v", ids1, ids2)
	}

	// Points should have same IDs
	if len(points1) != len(points2) {
		t.Fatalf("point count mismatch: %d vs %d", len(points1), len(points2))
	}
	for i := range points1 {
		if points1[i].ID != points2[i].ID {
			t.Errorf("point[%d] ID mismatch: %q vs %q", i, points1[i].ID, points2[i].ID)
		}
	}
}
