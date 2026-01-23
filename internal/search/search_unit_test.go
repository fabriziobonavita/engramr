package search

import (
	"context"
	"reflect"
	"testing"

	"github.com/fabriziobonavita/engramr/internal/store"
)

func TestSearcher_EmbedsQuery(t *testing.T) {
	embedder := newFakeEmbedder()
	fakeStore := newFakeStore()

	searcher := &Searcher{
		Collection: "test",
		Embedder:   embedder,
		Store:      fakeStore,
	}

	ctx := context.Background()
	queryText := "test query"

	// Set up search results
	fakeStore.setSearchResults("768-10", []store.SearchResult{
		{
			ID:    "id1",
			Score: 0.95,
			Payload: map[string]any{
				"source_path":  "test.md",
				"heading_path": []any{"Heading"},
				"content":      "test content",
			},
		},
	})

	_, err := searcher.Query(ctx, queryText, 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Verify embedder was called with query text
	embedded := embedder.GetEmbeddedText()
	found := false
	for _, text := range embedded {
		if text == queryText {
			found = true
			break
		}
	}
	if !found {
		t.Error("query text was not embedded")
	}
}

func TestSearcher_CallsStoreSearchWithTopK(t *testing.T) {
	embedder := newFakeEmbedder()
	fakeStore := newFakeStore()

	searcher := &Searcher{
		Collection: "test",
		Embedder:   embedder,
		Store:      fakeStore,
	}

	ctx := context.Background()
	queryText := "test query"
	topK := 5

	// Set up search results
	fakeStore.setSearchResults("768-5", []store.SearchResult{
		{
			ID:    "id1",
			Score: 0.95,
			Payload: map[string]any{
				"source_path":  "test.md",
				"heading_path": []any{"Heading"},
				"content":      "test content",
			},
		},
	})

	_, err := searcher.Query(ctx, queryText, topK)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Verify store.Search was called
	calls := fakeStore.getSearchCalls()
	if len(calls) == 0 {
		t.Fatal("store.Search was not called")
	}

	// Verify the call parameters
	call := calls[0]
	if call.collection != "test" {
		t.Errorf("collection = %q, want test", call.collection)
	}
	if call.topK != topK {
		t.Errorf("topK = %d, want %d", call.topK, topK)
	}
	if len(call.queryVector) == 0 {
		t.Error("queryVector is empty")
	}
}

func TestSearcher_FormatsHeadingPath(t *testing.T) {
	embedder := newFakeEmbedder()

	ctx := context.Background()

	tests := []struct {
		name        string
		headingPath []any
		wantPath    []string
	}{
		{
			name:        "empty heading path",
			headingPath: []any{},
			wantPath:    []string{},
		},
		{
			name:        "single heading",
			headingPath: []any{"Heading"},
			wantPath:    []string{"Heading"},
		},
		{
			name:        "nested headings",
			headingPath: []any{"Level 1", "Level 2", "Level 3"},
			wantPath:    []string{"Level 1", "Level 2", "Level 3"},
		},
		{
			name:        "non-string values ignored",
			headingPath: []any{"Heading", 123, "Subheading"},
			wantPath:    []string{"Heading", "Subheading"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create new store for each test to avoid state pollution
			fakeStore := newFakeStore()
			searcher := &Searcher{
				Collection: "test",
				Embedder:   embedder,
				Store:      fakeStore,
			}

			// Set up search results with specific heading path
			fakeStore.setSearchResults("768-10", []store.SearchResult{
				{
					ID:    "id1",
					Score: 0.95,
					Payload: map[string]any{
						"source_path":  "test.md",
						"heading_path": tt.headingPath,
						"content":      "test content",
					},
				},
			})

			fakeStore.setSearchResults("768-10", []store.SearchResult{
				{
					ID:    "id1",
					Score: 0.95,
					Payload: map[string]any{
						"source_path":  "test.md",
						"heading_path": tt.headingPath,
						"content":      "test content",
					},
				},
			})

			hits, err := searcher.Query(ctx, "test", 10)
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}

			if len(hits) == 0 {
				t.Fatal("no hits returned")
			}

			hit := hits[0]
			// Compare slices properly (handle nil vs empty)
			gotLen := len(hit.HeadingPath)
			wantLen := len(tt.wantPath)
			if gotLen != wantLen {
				t.Errorf("HeadingPath length = %d, want %d", gotLen, wantLen)
			} else if gotLen > 0 && !reflect.DeepEqual(hit.HeadingPath, tt.wantPath) {
				t.Errorf("HeadingPath = %v, want %v", hit.HeadingPath, tt.wantPath)
			} else if gotLen == 0 && wantLen == 0 {
				// Both empty, that's correct
			}
		})
	}
}

func TestSearcher_FormatsResults(t *testing.T) {
	embedder := newFakeEmbedder()
	fakeStore := newFakeStore()

	searcher := &Searcher{
		Collection: "test",
		Embedder:   embedder,
		Store:      fakeStore,
	}

	ctx := context.Background()

	// Set up search results with multiple hits
	fakeStore.setSearchResults("768-10", []store.SearchResult{
		{
			ID:    "id1",
			Score: 0.95,
			Payload: map[string]any{
				"source_path":  "test1.md",
				"heading_path": []any{"Heading 1"},
				"content":      "content 1",
			},
		},
		{
			ID:    "id2",
			Score: 0.85,
			Payload: map[string]any{
				"source_path":  "test2.md",
				"heading_path": []any{"Heading 2", "Subheading"},
				"content":      "content 2",
			},
		},
	})

	hits, err := searcher.Query(ctx, "test", 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}

	// Verify first hit
	if hits[0].SourcePath != "test1.md" {
		t.Errorf("hits[0].SourcePath = %q, want test1.md", hits[0].SourcePath)
	}
	if hits[0].Score != 0.95 {
		t.Errorf("hits[0].Score = %f, want 0.95", hits[0].Score)
	}
	if hits[0].Content != "content 1" {
		t.Errorf("hits[0].Content = %q, want content 1", hits[0].Content)
	}
	if !reflect.DeepEqual(hits[0].HeadingPath, []string{"Heading 1"}) {
		t.Errorf("hits[0].HeadingPath = %v, want [Heading 1]", hits[0].HeadingPath)
	}

	// Verify second hit
	if hits[1].SourcePath != "test2.md" {
		t.Errorf("hits[1].SourcePath = %q, want test2.md", hits[1].SourcePath)
	}
	if !reflect.DeepEqual(hits[1].HeadingPath, []string{"Heading 2", "Subheading"}) {
		t.Errorf("hits[1].HeadingPath = %v, want [Heading 2 Subheading]", hits[1].HeadingPath)
	}
}

func TestSnippet(t *testing.T) {
	tests := []struct {
		name    string
		content string
		limit   int
		want    string
	}{
		{
			name:    "content shorter than limit",
			content: "short content",
			limit:   100,
			want:    "short content",
		},
		{
			name:    "content longer than limit",
			content: "This is a very long content that exceeds the limit and should be truncated",
			limit:   20,
			want:    "This is a very long ", // 20 chars includes the space after "long"
		},
		{
			name:    "normalizes whitespace",
			content: "This   has    multiple\n\nspaces   and\n\nnewlines",
			limit:   100,
			want:    "This has multiple spaces and newlines",
		},
		{
			name:    "zero limit uses default",
			content: "content",
			limit:   0,
			want:    "content",
		},
		{
			name:    "negative limit uses default",
			content: "content",
			limit:   -1,
			want:    "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Snippet(tt.content, tt.limit)
			if got != tt.want {
				t.Errorf("Snippet(%q, %d) = %q, want %q", tt.content, tt.limit, got, tt.want)
			}
		})
	}
}
