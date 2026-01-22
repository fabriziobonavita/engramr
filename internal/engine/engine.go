package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/fabriziobonavita/engramr/internal/embed"
	"github.com/fabriziobonavita/engramr/internal/store"
)

const (
	DefaultQdrantURL        = "http://localhost:6334"
	DefaultCollection       = "engramr_notes_v0"
	DefaultOllamaURL        = "http://localhost:11434"
	DefaultEmbeddingModel   = "nomic-embed-text"
	DefaultSnippetCharLimit = 200
	DefaultManifestPath     = ".engramr/index.json"
)

type Engine struct {
	Collection   string
	ManifestPath string // Path to manifest file. If empty, uses DefaultManifestPath.
	Embedder     *embed.OllamaClient
	Store        *store.QdrantClient
}

func NewDefault() *Engine {
	return New(DefaultCollection)
}

// New creates an Engine with a custom collection name.
// This is useful for tests that need isolated collections.
func New(collection string) *Engine {
	return &Engine{
		Collection: collection,
		Embedder: &embed.OllamaClient{
			BaseURL: DefaultOllamaURL,
			Model:   DefaultEmbeddingModel,
		},
		Store: &store.QdrantClient{
			BaseURL: DefaultQdrantURL,
		},
	}
}

func (e *Engine) Init(ctx context.Context) error {
	vec, err := e.Embedder.Embed(ctx, "engramr vector size probe")
	if err != nil {
		return err
	}
	if len(vec) == 0 {
		return fmt.Errorf("ollama returned empty embedding vector")
	}
	return e.Store.EnsureCollection(ctx, e.Collection, len(vec))
}

type QueryHit struct {
	SourcePath  string
	HeadingPath []string
	Score       float64
	Content     string
}

func (e *Engine) Query(ctx context.Context, text string, topK int) ([]QueryHit, error) {
	if err := e.Init(ctx); err != nil {
		return nil, err
	}

	vec, err := e.Embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	results, err := e.Store.Search(ctx, e.Collection, vec, topK)
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
