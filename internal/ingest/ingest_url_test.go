package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabriziobonavita/engramr/internal/extract"
	"github.com/fabriziobonavita/engramr/internal/indexstate"
	"github.com/fabriziobonavita/engramr/internal/testutil"
)

func TestIngestURL_Deduplication(t *testing.T) {
	// Create HTML content
	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Test Article for Deduplication</title>
</head>
<body>
	<main>
		<article>
			<h1>Main Article</h1>
			<p>This is the main content that should be extracted and indexed. This paragraph contains important information about the topic and provides context for understanding the subject matter.</p>
			<p>This content should only be indexed once, even if we ingest the URL twice. This ensures that duplicate content is not stored multiple times in the index.</p>
			<p>Here is another paragraph that adds more content to ensure readability can successfully extract the main content. This helps to meet the minimum content requirements for extraction.</p>
			<p>Yet another paragraph continues the discussion with additional information. This content provides depth and context to the article, making it more comprehensive and useful.</p>
			<p>This paragraph adds even more content to ensure we have sufficient text for the extraction process. The goal is to provide a complete example that demonstrates the deduplication functionality.</p>
			<p>Finally, this last paragraph wraps up the content with concluding thoughts and information. Together, all these paragraphs should provide enough text for readability to successfully extract the main content.</p>
		</article>
	</main>
</body>
</html>`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	// Create temporary directory for index state
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, ".engramr")

	// Create fake embedder and store
	embedder := testutil.NewFakeEmbedder()
	store := newFakeStore()

	ing := &Ingestor{
		Collection:        "test_collection",
		IndexStateBaseDir: baseDir,
		Embedder:          embedder,
		Store:             store,
		IndexState:        indexstate.NewFileStore(),
	}

	// First ingest
	sum1, err := ing.IngestURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("First IngestURL failed: %v", err)
	}

	if sum1.FilesIngested != 1 {
		t.Errorf("Expected 1 file ingested on first run, got %d", sum1.FilesIngested)
	}

	// Get initial point count
	initialPointCount := len(store.getPoints("test_collection"))

	// Second ingest (should be deduplicated)
	sum2, err := ing.IngestURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Second IngestURL failed: %v", err)
	}

	// Should not ingest again (deduplicated)
	if sum2.FilesIngested != 0 {
		t.Errorf("Expected 0 files ingested on second run (deduplicated), got %d", sum2.FilesIngested)
	}

	// Should not have upserted any new points
	finalPointCount := len(store.getPoints("test_collection"))
	if finalPointCount != initialPointCount {
		t.Errorf("Expected no new points on second ingest, but point count changed from %d to %d", initialPointCount, finalPointCount)
	}

	// Verify index state has the entry with correct content hash
	indexStore := indexstate.NewFileStore()
	state, err := indexStore.Load(baseDir)
	if err != nil {
		t.Fatalf("Failed to load index state: %v", err)
	}

	entry, exists := state.GetFile(server.URL)
	if !exists {
		t.Fatal("Expected index state entry for URL, but it doesn't exist")
	}

	if entry.ContentHash == "" {
		t.Error("Expected ContentHash to be set in index state entry")
	}

	// Verify content hash matches what we'd get from extraction
	urlContent, err := extract.ExtractURL(server.URL, "engramr/test")
	if err != nil {
		t.Fatalf("Failed to extract URL for hash verification: %v", err)
	}

	if entry.ContentHash != urlContent.ContentHash {
		t.Errorf("ContentHash mismatch: index state has %s, extracted content has %s", entry.ContentHash, urlContent.ContentHash)
	}
}

func TestIngestURL_ContentChange(t *testing.T) {
	// Test that changed content is re-ingested
	htmlContent1 := `<!DOCTYPE html>
<html>
<head>
	<title>Original Article</title>
</head>
<body>
	<main>
		<article>
			<h1>Original Article Title</h1>
			<p>Original content that will be extracted and indexed. This paragraph contains the initial information about the topic.</p>
			<p>Here is another paragraph with more original content. This helps to ensure there is enough text for readability to successfully extract the content.</p>
			<p>This paragraph adds additional original information to the article. The goal is to provide sufficient content for the extraction process to work correctly.</p>
			<p>Yet another paragraph continues with more original content. This ensures that the article has enough depth and substance for successful extraction.</p>
			<p>Finally, this last paragraph wraps up the original content with concluding thoughts. Together, all these paragraphs should provide enough text for readability extraction.</p>
		</article>
	</main>
</body>
</html>`

	htmlContent2 := `<!DOCTYPE html>
<html>
<head>
	<title>Updated Article</title>
</head>
<body>
	<main>
		<article>
			<h1>Updated Article Title</h1>
			<p>Updated content with new information that replaces the original content. This paragraph contains revised information about the topic.</p>
			<p>Here is another paragraph with more updated content. This helps to ensure there is enough text for readability to successfully extract the updated content.</p>
			<p>This paragraph adds additional updated information to the article. The goal is to provide sufficient content for the extraction process to work correctly with the new content.</p>
			<p>Yet another paragraph continues with more updated content. This ensures that the article has enough depth and substance for successful extraction of the revised information.</p>
			<p>Finally, this last paragraph wraps up the updated content with concluding thoughts. Together, all these paragraphs should provide enough text for readability extraction of the new content.</p>
		</article>
	</main>
</body>
</html>`

	contentVersion := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if contentVersion == 1 {
			_, _ = w.Write([]byte(htmlContent1))
		} else {
			_, _ = w.Write([]byte(htmlContent2))
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, ".engramr")

	embedder := testutil.NewFakeEmbedder()
	store := newFakeStore()

	ing := &Ingestor{
		Collection:        "test_collection",
		IndexStateBaseDir: baseDir,
		Embedder:          embedder,
		Store:             store,
		IndexState:        indexstate.NewFileStore(),
	}

	// First ingest
	sum1, err := ing.IngestURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("First IngestURL failed: %v", err)
	}

	if sum1.FilesIngested != 1 {
		t.Errorf("Expected 1 file ingested, got %d", sum1.FilesIngested)
	}

	initialPointCount := len(store.getPoints("test_collection"))

	// Change content and ingest again
	contentVersion = 2
	sum2, err := ing.IngestURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Second IngestURL failed: %v", err)
	}

	// Should ingest again because content changed
	if sum2.FilesIngested != 1 {
		t.Errorf("Expected 1 file ingested on second run (content changed), got %d", sum2.FilesIngested)
	}

	// Should have upserted new points (old ones deleted, new ones added)
	// The exact count depends on chunking, but should be different
	finalPointCount := len(store.getPoints("test_collection"))
	if finalPointCount == initialPointCount {
		t.Logf("Point count unchanged (%d), which might be OK if chunking produces same number of chunks", initialPointCount)
	}
}

func TestIngestURL_RedirectCanonicalURL(t *testing.T) {
	// Test that redirects are followed and canonical URL is stored
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirectCount == 0 {
			redirectCount++
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Final Page</title>
</head>
<body>
	<main>
		<article>
			<h1>Final Content</h1>
			<p>This is the final content after redirect. This paragraph contains important information that should be extracted by readability.</p>
			<p>Here is another paragraph with more detailed information about the topic. This helps to ensure there is enough content for successful extraction.</p>
			<p>This paragraph adds additional context and information to the article. The goal is to provide sufficient text for the extraction process.</p>
			<p>Yet another paragraph continues the discussion with more comprehensive information. This ensures that the content has enough depth for readability extraction.</p>
			<p>Finally, this last paragraph wraps up the final content with concluding thoughts. Together, all these paragraphs should provide enough text for successful extraction.</p>
		</article>
	</main>
</body>
</html>`
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, ".engramr")

	embedder := testutil.NewFakeEmbedder()
	store := newFakeStore()

	ing := &Ingestor{
		Collection:        "test_collection",
		IndexStateBaseDir: baseDir,
		Embedder:          embedder,
		Store:             store,
		IndexState:        indexstate.NewFileStore(),
	}

	sum, err := ing.IngestURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("IngestURL failed: %v", err)
	}

	if sum.FilesIngested != 1 {
		t.Errorf("Expected 1 file ingested, got %d", sum.FilesIngested)
	}

	// Verify that canonical URL (after redirect) is stored in index state
	indexStore := indexstate.NewFileStore()
	state, err := indexStore.Load(baseDir)
	if err != nil {
		t.Fatalf("Failed to load index state: %v", err)
	}

	expectedCanonical := server.URL + "/final"
	entry, exists := state.GetFile(expectedCanonical)
	if !exists {
		t.Fatalf("Expected index state entry for canonical URL %s, but it doesn't exist. Available keys: %v", expectedCanonical, getStateKeys(state))
	}

	// Verify points have canonical URL in payload
	foundCanonical := false
	for _, point := range store.getPoints("test_collection") {
		if canonicalURL, ok := point.Payload["canonical_url"].(string); ok {
			if canonicalURL == expectedCanonical {
				foundCanonical = true
				break
			}
		}
	}

	if !foundCanonical {
		t.Error("Expected to find canonical_url in point payloads, but didn't")
	}

	// Verify entry exists
	if len(entry.PointIDs) == 0 {
		t.Error("Expected point IDs in index state entry")
	}
}

// Helper to get state keys for debugging
func getStateKeys(state *indexstate.IndexState) []string {
	if state == nil || state.Files == nil {
		return []string{}
	}
	keys := make([]string, 0, len(state.Files))
	for k := range state.Files {
		keys = append(keys, k)
	}
	return keys
}

func TestIngestURL_Metadata(t *testing.T) {
	// Test that URL metadata is properly stored in point payloads
	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Test Article Metadata</title>
</head>
<body>
	<main>
		<article>
			<h1>Test Article for Metadata</h1>
			<p>Content for metadata test. This paragraph contains important information that should be extracted and stored with proper metadata.</p>
			<p>Here is another paragraph with more content to ensure readability can successfully extract the main content. This helps to meet the minimum content requirements.</p>
			<p>This paragraph adds additional information to the article. The goal is to provide sufficient text for the extraction process to work correctly.</p>
			<p>Yet another paragraph continues the discussion with more comprehensive information. This ensures that the article has enough depth and substance for successful extraction.</p>
			<p>Finally, this last paragraph wraps up the content with concluding thoughts. Together, all these paragraphs should provide enough text for readability to successfully extract the main content.</p>
		</article>
	</main>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, ".engramr")

	embedder := testutil.NewFakeEmbedder()
	store := newFakeStore()

	ing := &Ingestor{
		Collection:        "test_collection",
		IndexStateBaseDir: baseDir,
		Embedder:          embedder,
		Store:             store,
		IndexState:        indexstate.NewFileStore(),
	}

	_, err := ing.IngestURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("IngestURL failed: %v", err)
	}

	// Verify metadata in at least one point
	points := store.getPoints("test_collection")
	if len(points) == 0 {
		t.Fatal("Expected at least one point to be upserted")
	}

	point := points[0]
	payload := point.Payload

	// Check required metadata fields
	requiredFields := []string{"source", "canonical_url", "title", "fetched_at", "content_type", "content_hash", "type"}
	for _, field := range requiredFields {
		if _, ok := payload[field]; !ok {
			t.Errorf("Expected payload to contain field '%s', but it doesn't", field)
		}
	}

	// Verify specific values
	if payload["source"] != "url" {
		t.Errorf("Expected source='url', got '%v'", payload["source"])
	}

	if payload["type"] != "url" {
		t.Errorf("Expected type='url', got '%v'", payload["type"])
	}

	canonicalURL, ok := payload["canonical_url"].(string)
	if !ok {
		t.Error("Expected canonical_url to be a string")
	} else if canonicalURL != server.URL {
		t.Errorf("Expected canonical_url=%s, got %s", server.URL, canonicalURL)
	}

	title, ok := payload["title"].(string)
	if !ok {
		t.Error("Expected title to be a string")
	} else if !strings.Contains(title, "Test Article") {
		t.Errorf("Expected title to contain 'Test Article', got '%s'", title)
	}

	// Verify fetched_at is RFC3339 format
	fetchedAt, ok := payload["fetched_at"].(string)
	if !ok {
		t.Error("Expected fetched_at to be a string")
	} else {
		// Basic RFC3339 format check (contains T and Z or timezone)
		if !strings.Contains(fetchedAt, "T") {
			t.Errorf("Expected fetched_at in RFC3339 format, got '%s'", fetchedAt)
		}
	}
}
