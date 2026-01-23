package ingest

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

	"github.com/fabriziobonavita/engramr/internal/extract"
	"github.com/fabriziobonavita/engramr/internal/manifest"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/google/uuid"
)

const (
	DefaultManifestPath = ".engramr/index.json"
)

// Namespace UUID for deterministic point ID generation.
// This is a fixed UUID used as the namespace for SHA1-based UUID generation.
var pointIDNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

// Embedder is an interface for embedding text into vectors.
type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
}

// Store is an interface for vector store operations.
type Store interface {
	EnsureCollection(ctx context.Context, name string, vectorSize int) error
	UpsertPoints(ctx context.Context, collection string, points []store.Point) error
	DeletePoints(ctx context.Context, collection string, pointIDs []string) error
}

// ManifestStore is an interface for manifest operations.
type ManifestStore interface {
	Load(path string) (*manifest.Manifest, error)
	Save(path string, m *manifest.Manifest) error
}

// Ingestor orchestrates extract+embed+store+manifest for ingestion.
type Ingestor struct {
	Collection   string
	ManifestPath string // Path to manifest file. If empty, uses DefaultManifestPath.
	Embedder     Embedder
	Store        Store
	Manifest     ManifestStore
}

// IngestSummary reports the results of an ingest operation.
type IngestSummary struct {
	FilesIngested  int
	ChunksUpserted int
	ChunksDeleted  int
	ChunksSkipped  int
	Errors         int
}

// Init ensures the collection exists (requires embedder reachable).
func (i *Ingestor) Init(ctx context.Context) error {
	vec, err := i.Embedder.Embed(ctx, "engramr vector size probe")
	if err != nil {
		return err
	}
	if len(vec) == 0 {
		return fmt.Errorf("ollama returned empty embedding vector")
	}
	return i.Store.EnsureCollection(ctx, i.Collection, len(vec))
}

// IngestPath ingests markdown files from the given path, updating the index and manifest.
func (i *Ingestor) IngestPath(ctx context.Context, path string) (IngestSummary, error) {
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
	if err := i.Init(ctx); err != nil {
		return sum, err
	}

	// Load manifest
	manifestPath := i.ManifestPath
	if manifestPath == "" {
		manifestPath = DefaultManifestPath
	}
	manifestStore := i.Manifest
	if manifestStore == nil {
		manifestStore = &fileManifestStore{}
	}
	m, err := manifestStore.Load(manifestPath)
	if err != nil {
		return sum, fmt.Errorf("failed to load manifest: %w", err)
	}

	mdFiles := findMarkdownFiles(absRoot, stat)
	log.Printf("found %d markdown files under %s", len(mdFiles), absRoot)

	// Upsert in batches to keep payload sizes reasonable.
	const batchSize = 64
	var batch []store.Point

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := i.Store.UpsertPoints(ctx, i.Collection, batch); err != nil {
			return err
		}
		sum.ChunksUpserted += len(batch)
		batch = batch[:0]
		return nil
	}

	// Process each file
	for _, filePath := range mdFiles {
		if err := i.processFile(ctx, filePath, rootDir, m, &sum, &batch, batchSize, flush); err != nil {
			sum.Errors++
			log.Printf("error processing file %s: %v", filePath, err)
			// Continue with other files
		}
	}

	// Flush remaining batch
	if err := flush(); err != nil {
		sum.Errors++
		log.Printf("error flushing final batch: %v", err)
	}

	// Save manifest
	if err := manifestStore.Save(manifestPath, m); err != nil {
		return sum, fmt.Errorf("failed to save manifest: %w", err)
	}

	return sum, nil
}

// processFile processes a single markdown file for ingestion.
func (i *Ingestor) processFile(
	ctx context.Context,
	filePath string,
	rootDir string,
	m *manifest.Manifest,
	sum *IngestSummary,
	batch *[]store.Point,
	batchSize int,
	flush func() error,
) error {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("statting file: %w", err)
	}

	rel, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		rel = filePath
	}
	rel = filepath.ToSlash(rel)

	chunks := extract.ChunkMarkdown(rel, string(b))
	if len(chunks) == 0 {
		// Remove file from manifest if it has no chunks
		m.SetFile(rel, manifest.FileEntry{
			Mtime:    info.ModTime().Unix(),
			PointIDs: []string{},
		})
		return nil
	}

	sum.FilesIngested++

	// Process chunks: compute IDs, embed, build points
	newPointIDs, pointsToUpsert, err := i.processChunks(ctx, chunks, info.ModTime().Unix(), sum)
	if err != nil {
		return fmt.Errorf("processing chunks: %w", err)
	}

	// Get old point IDs from manifest
	oldEntry, _ := m.GetFile(rel)

	// Delete stale points (old - new)
	deleteIDs := diffIds(oldEntry.PointIDs, newPointIDs)
	if len(deleteIDs) > 0 {
		if err := i.Store.DeletePoints(ctx, i.Collection, deleteIDs); err != nil {
			log.Printf("error deleting stale points for %s: %v", rel, err)
		} else {
			sum.ChunksDeleted += len(deleteIDs)
		}
	}

	// Upsert only new/changed points (new - old)
	upsertIDs := diffIds(newPointIDs, oldEntry.PointIDs)
	upsertIDsSet := make(map[string]struct{})
	for _, id := range upsertIDs {
		upsertIDsSet[id] = struct{}{}
	}

	for _, p := range pointsToUpsert {
		if _, shouldUpsert := upsertIDsSet[p.ID]; shouldUpsert {
			*batch = append(*batch, p)
			if len(*batch) >= batchSize {
				if err := flush(); err != nil {
					return fmt.Errorf("flushing batch: %w", err)
				}
			}
		} else {
			sum.ChunksSkipped++
		}
	}

	// Update manifest entry for this file
	m.SetFile(rel, manifest.FileEntry{
		Mtime:    info.ModTime().Unix(),
		PointIDs: newPointIDs,
	})

	return nil
}

// processChunks processes chunks: embeds content, computes point IDs, and builds store points.
func (i *Ingestor) processChunks(ctx context.Context, chunks []extract.Chunk, mtime int64, sum *IngestSummary) ([]string, []store.Point, error) {
	var newPointIDs []string
	var pointsToUpsert []store.Point

	for _, c := range chunks {
		vec, err := i.Embedder.Embed(ctx, c.Content)
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
			"modified_time": mtime,
			"type":          "md",
			"content":       c.Content,
		}

		pointsToUpsert = append(pointsToUpsert, store.Point{
			ID:      pointID,
			Vector:  vec,
			Payload: payload,
		})
	}

	return newPointIDs, pointsToUpsert, nil
}

// findMarkdownFiles finds all markdown files in the given path.
func findMarkdownFiles(absRoot string, stat os.FileInfo) []string {
	var mdFiles []string
	if stat.IsDir() {
		filepath.WalkDir(absRoot, func(p string, d fs.DirEntry, walkErr error) error {
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
	} else {
		if strings.HasSuffix(strings.ToLower(absRoot), ".md") {
			mdFiles = append(mdFiles, absRoot)
		}
	}
	return mdFiles
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

	var result []string
	for _, id := range oldIDs {
		if _, ok := newIDsSet[id]; !ok {
			result = append(result, id)
		}
	}
	return result
}

// sha1Hex computes the SHA1 hash of the input and returns it as a hex string.
func sha1Hex(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

// fileManifestStore implements ManifestStore using the file system.
type fileManifestStore struct{}

func (f *fileManifestStore) Load(path string) (*manifest.Manifest, error) {
	return manifest.Load(path)
}

func (f *fileManifestStore) Save(path string, m *manifest.Manifest) error {
	return manifest.Save(path, m)
}
