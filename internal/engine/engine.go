package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fabriziobonavita/engramr/internal/chunk"
	"github.com/fabriziobonavita/engramr/internal/embed"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/google/uuid"
)

const (
	DefaultQdrantURL        = "http://localhost:6334"
	DefaultCollection       = "engramr_notes_v0"
	DefaultOllamaURL        = "http://localhost:11434"
	DefaultEmbeddingModel   = "nomic-embed-text"
	DefaultSnippetCharLimit = 200
)

type Engine struct {
	Collection string
	Embedder   *embed.OllamaClient
	Store      *store.QdrantClient
}

func NewDefault() *Engine {
	return &Engine{
		Collection: DefaultCollection,
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

type IngestSummary struct {
	FilesIngested  int
	ChunksUpserted int
}

func (e *Engine) IngestPath(ctx context.Context, path string) (IngestSummary, error) {
	var sum IngestSummary

	absRoot, err := filepath.Abs(path)
	if err != nil {
		return sum, err
	}

	stat, err := os.Stat(absRoot)
	if err != nil {
		return sum, err
	}

	rootDir := absRoot
	if !stat.IsDir() {
		rootDir = filepath.Dir(absRoot)
	}

	// Ensure collection exists (requires embedder reachable).
	if err := e.Init(ctx); err != nil {
		return sum, err
	}

	var mdFiles []string
	if stat.IsDir() {
		err = filepath.WalkDir(absRoot, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				mdFiles = append(mdFiles, p)
			}
			return nil
		})
		if err != nil {
			return sum, err
		}
	} else {
		if strings.HasSuffix(strings.ToLower(absRoot), ".md") {
			mdFiles = append(mdFiles, absRoot)
		}
	}

	log.Printf("found %d markdown files under %s", len(mdFiles), absRoot)

	// Upsert in batches to keep payload sizes reasonable.
	const batchSize = 64
	var batch []store.Point

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := e.Store.UpsertPoints(ctx, e.Collection, batch); err != nil {
			return err
		}
		sum.ChunksUpserted += len(batch)
		batch = batch[:0]
		return nil
	}

	for _, filePath := range mdFiles {
		b, err := os.ReadFile(filePath)
		if err != nil {
			return sum, err
		}
		info, err := os.Stat(filePath)
		if err != nil {
			return sum, err
		}

		rel, err := filepath.Rel(rootDir, filePath)
		if err != nil {
			rel = filePath
		}
		rel = filepath.ToSlash(rel)

		chunks := chunk.ChunkMarkdown(rel, string(b))
		if len(chunks) == 0 {
			continue
		}

		sum.FilesIngested++

		for _, c := range chunks {
			vec, err := e.Embedder.Embed(ctx, c.Content)
			if err != nil {
				return sum, err
			}

			// Skip chunks with empty vectors
			if len(vec) == 0 {
				log.Printf("warning: empty embedding for chunk in %s, skipping", c.SourcePath)
				continue
			}

			contentHash := sha1Hex([]byte(c.Content))
			chunkID := sha1Hex([]byte(c.SourcePath + "|" + strings.Join(c.HeadingPath, ">") + "|" + strconv.Itoa(c.ChunkIndex) + "|" + contentHash))

			// Generate UUID for Qdrant ID
			pointID := uuid.New().String()

			// Convert heading_path from []string to []any for Qdrant compatibility
			headingPathAny := make([]any, len(c.HeadingPath))
			for i, h := range c.HeadingPath {
				headingPathAny[i] = h
			}

			payload := map[string]any{
				"chunk_id":      chunkID,
				"source_path":   c.SourcePath,
				"heading_path":  headingPathAny,
				"chunk_index":   c.ChunkIndex,
				"content_hash":  contentHash,
				"modified_time": info.ModTime().Unix(),
				"type":          "md",
				"content":       c.Content,
			}

			batch = append(batch, store.Point{
				ID:      pointID,
				Vector:  vec,
				Payload: payload,
			})

			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					return sum, err
				}
			}
		}
	}

	if err := flush(); err != nil {
		return sum, err
	}

	return sum, nil
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

func sha1Hex(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}
