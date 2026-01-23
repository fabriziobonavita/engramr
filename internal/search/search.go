package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/fabriziobonavita/engramr/internal/embed"
	"github.com/fabriziobonavita/engramr/internal/store"
)

const (
	DefaultQdrantURL        = "http://localhost:6334"
	DefaultCollection      = "engramr_notes_v0"
	DefaultOllamaURL       = "http://localhost:11434"
	DefaultEmbeddingModel  = "nomic-embed-text"
	DefaultSnippetCharLimit = 200
)

// Searcher orchestrates embed+store and returns results.
type Searcher struct {
	Collection string
	Embedder   *embed.OllamaClient
	Store      *store.QdrantClient
}

// QueryHit represents a single search result.
type QueryHit struct {
	SourcePath  string
	HeadingPath []string
	Score       float64
	Content     string
}

// Init ensures the collection exists (requires embedder reachable).
func (s *Searcher) Init(ctx context.Context) error {
	vec, err := s.Embedder.Embed(ctx, "engramr vector size probe")
	if err != nil {
		return err
	}
	if len(vec) == 0 {
		return fmt.Errorf("ollama returned empty embedding vector")
	}
	return s.Store.EnsureCollection(ctx, s.Collection, len(vec))
}

// Query searches for semantically similar passages.
func (s *Searcher) Query(ctx context.Context, text string, topK int) ([]QueryHit, error) {
	if err := s.Init(ctx); err != nil {
		return nil, err
	}

	vec, err := s.Embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	results, err := s.Store.Search(ctx, s.Collection, vec, topK)
	if err != nil {
		return nil, err
	}

	hits := make([]QueryHit, 0, len(results))
	for _, r := range results {
		hit := QueryHit{
			Score: r.Score,
		}

		if v, ok := r.Payload["source_path"].(string); ok {
			hit.SourcePath = v
		}
		if v, ok := r.Payload["content"].(string); ok {
			hit.Content = v
		}
		if v, ok := r.Payload["heading_path"].([]any); ok {
			for _, x := range v {
				if s, ok := x.(string); ok {
					hit.HeadingPath = append(hit.HeadingPath, s)
				}
			}
		}

		hits = append(hits, hit)
	}

	return hits, nil
}

// Snippet normalizes and truncates content to a character limit.
func Snippet(content string, limit int) string {
	normalized := strings.Join(strings.Fields(content), " ")
	if limit <= 0 {
		limit = DefaultSnippetCharLimit
	}
	if len(normalized) <= limit {
		return normalized
	}
	return normalized[:limit]
}
