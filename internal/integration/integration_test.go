//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fabriziobonavita/engramr/internal/engine"
)

const (
	sentinelPhrase = "ENGRAMR_INTEGRATION_SENTINEL_PHRASE_12345"
	qdrantURL      = "http://localhost:6333"
	ollamaURL      = "http://localhost:11434"
)

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client := &http.Client{Timeout: 5 * time.Second}

	// Check service reachability
	if err := checkReachable(ctx, client, qdrantURL+"/collections"); err != nil {
		t.Skipf("Qdrant not reachable: %v", err)
	}
	if err := checkReachable(ctx, client, ollamaURL+"/api/tags"); err != nil {
		t.Skipf("Ollama not reachable: %v", err)
	}

	// Find testdata relative to test file location
	_, testFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(testFile)
	testDataPath := filepath.Join(testDir, "..", "..", "testdata", "notes")
	testDataPath, err := filepath.Abs(testDataPath)
	if err != nil {
		t.Fatalf("failed to resolve testdata path: %v", err)
	}

	// Use temp directory for manifest
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "index.json")

	// Create unique collection name
	collectionName := fmt.Sprintf("engramr_test_%d", time.Now().UnixNano())
	eng := engine.New(collectionName)
	eng.ManifestPath = manifestPath

	// Cleanup: delete collection at end (best effort)
	defer func() {
		if err := eng.Store.DeleteCollection(ctx, collectionName); err != nil {
			t.Logf("warning: failed to cleanup test collection %s: %v", collectionName, err)
		}
	}()

	// Initialize collection
	if err := eng.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Ingest test fixture
	sum, err := eng.IngestPath(ctx, testDataPath)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if sum.FilesIngested == 0 {
		t.Fatal("no files were ingested")
	}
	if sum.Errors > 0 {
		t.Fatalf("ingest had %d errors", sum.Errors)
	}

	// Query for sentinel phrase
	hits, err := eng.Query(ctx, sentinelPhrase, 5)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("query returned no results")
	}

	// Assert exact source_path match
	found := false
	for _, hit := range hits {
		if hit.SourcePath == "sample.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find result with source_path 'sample.md'")
		for i, hit := range hits {
			t.Logf("  %d) %s (score: %.4f)", i+1, hit.SourcePath, hit.Score)
		}
		t.FailNow()
	}
}

func checkReachable(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
