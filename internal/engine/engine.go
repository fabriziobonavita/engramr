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
	"github.com/fabriziobonavita/engramr/internal/manifest"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/google/uuid"
)

// Namespace UUID for deterministic point ID generation.
// This is a fixed UUID used as the namespace for SHA1-based UUID generation.
var pointIDNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

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
	ChunksDeleted  int
	Errors         int
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

	// Load manifest
	manifestPath := ".engramr/index.json"
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return sum, fmt.Errorf("failed to load manifest: %w", err)
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

	// Process each file
	for _, filePath := range mdFiles {
		b, err := os.ReadFile(filePath)
		if err != nil {
			sum.Errors++
			log.Printf("error reading file %s: %v", filePath, err)
			continue
		}
		info, err := os.Stat(filePath)
		if err != nil {
			sum.Errors++
			log.Printf("error statting file %s: %v", filePath, err)
			continue
		}

		rel, err := filepath.Rel(rootDir, filePath)
		if err != nil {
			rel = filePath
		}
		rel = filepath.ToSlash(rel)

		chunks := chunk.ChunkMarkdown(rel, string(b))
		if len(chunks) == 0 {
			// Remove file from manifest if it has no chunks
			m.SetFile(rel, manifest.FileEntry{
				Mtime:    info.ModTime().Unix(),
				PointIDs: []string{},
			})
			continue
		}

		sum.FilesIngested++

		// Compute deterministic point IDs for all chunks
		var newPointIDs []string
		var pointsToUpsert []store.Point

		for _, c := range chunks {
			vec, err := e.Embedder.Embed(ctx, c.Content)
			if err != nil {
				sum.Errors++
				log.Printf("error embedding chunk in %s: %v", c.SourcePath, err)
				continue
			}

			// Skip chunks with empty vectors
			if len(vec) == 0 {
				log.Printf("warning: empty embedding for chunk in %s, skipping", c.SourcePath)
				continue
			}

			contentHash := sha1Hex([]byte(c.Content))
			// Build chunkKey: source_path + "\n" + heading_path + "\n" + chunk_index + "\n" + content_hash
			chunkKey := c.SourcePath + "\n" + strings.Join(c.HeadingPath, "/") + "\n" + strconv.Itoa(c.ChunkIndex) + "\n" + contentHash
			pointID := PointIDFromChunkKey(chunkKey)
			newPointIDs = append(newPointIDs, pointID)

			// Convert heading_path from []string to []any for Qdrant compatibility
			headingPathAny := make([]any, len(c.HeadingPath))
			for i, h := range c.HeadingPath {
				headingPathAny[i] = h
			}

			payload := map[string]any{
				"source_path":   c.SourcePath,
				"heading_path":  headingPathAny,
				"chunk_index":   c.ChunkIndex,
				"content_hash":  contentHash,
				"modified_time": info.ModTime().Unix(),
				"type":          "md",
				"content":       c.Content,
			}

			pointsToUpsert = append(pointsToUpsert, store.Point{
				ID:      pointID,
				Vector:  vec,
				Payload: payload,
			})
		}

		// Get old point IDs from manifest
		oldEntry, _ := m.GetFile(rel)

		deleteIDs := diffIds(oldEntry.PointIDs, newPointIDs)

		// Delete stale points (best-effort)
		if len(deleteIDs) > 0 {
			if err := e.Store.DeletePoints(ctx, e.Collection, deleteIDs); err != nil {
				sum.Errors++
				log.Printf("error deleting stale points for %s: %v", rel, err)
			} else {
				sum.ChunksDeleted += len(deleteIDs)
			}
		}

		// Upsert new points
		for _, p := range pointsToUpsert {
			batch = append(batch, p)
			if len(batch) >= batchSize {
				if err := flush(); err != nil {
					sum.Errors++
					log.Printf("error upserting batch: %v", err)
					// Continue processing other files
				}
			}
		}

		// Update manifest entry for this file
		m.SetFile(rel, manifest.FileEntry{
			Mtime:    info.ModTime().Unix(),
			PointIDs: newPointIDs,
		})
	}

	// Flush remaining batch
	if err := flush(); err != nil {
		sum.Errors++
		log.Printf("error flushing final batch: %v", err)
	}

	// Save manifest
	if err := manifest.Save(manifestPath, m); err != nil {
		return sum, fmt.Errorf("failed to save manifest: %w", err)
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

// PointIDFromChunkKey generates a deterministic UUID string from a chunk key.
// The chunkKey format is:
//
//	source_path + "\n" + strings.Join(heading_path, "/") + "\n" + strconv.Itoa(chunk_index) + "\n" + content_hash
func PointIDFromChunkKey(chunkKey string) string {
	return uuid.NewSHA1(pointIDNamespace, []byte(chunkKey)).String()
}

// diffIds returns IDs that are in oldIDs but not in newIDs (old - new).
func diffIds(oldIDs, newIDs []string) []string {
	newIDsSet := make(map[string]struct{})
	for _, id := range newIDs {
		newIDsSet[id] = struct{}{}
	}

	var deleteIDs []string
	for _, id := range oldIDs {
		if _, ok := newIDsSet[id]; !ok {
			deleteIDs = append(deleteIDs, id)
		}
	}
	return deleteIDs
}
