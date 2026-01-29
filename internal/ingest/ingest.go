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
	"unicode/utf8"

	"github.com/fabriziobonavita/engramr/internal/buildinfo"
	"github.com/fabriziobonavita/engramr/internal/extract"
	"github.com/fabriziobonavita/engramr/internal/indexstate"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/google/uuid"
)

const (
	DefaultIndexStateBaseDir = ".engramr"
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

// Ingestor orchestrates extract+embed+store+indexstate for ingestion.
type Ingestor struct {
	Collection        string
	IndexStateBaseDir string // Base directory for index state. If empty, uses DefaultIndexStateBaseDir.
	Embedder          Embedder
	Store             Store
	IndexState        indexstate.Store // Index state store. If nil, uses a default file-based store.
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

// IngestPath ingests markdown files from the given path, updating the index and index state.
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

	// Get index state base directory
	baseDir := i.IndexStateBaseDir
	if baseDir == "" {
		baseDir = DefaultIndexStateBaseDir
	}
	indexStore := i.IndexState
	if indexStore == nil {
		indexStore = indexstate.NewFileStore()
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

	// Use transaction for entire read-modify-write cycle
	err = indexStore.Transaction(baseDir, func(state *indexstate.IndexState) error {
		// Process each file
		for _, filePath := range mdFiles {
			if err := i.processFile(ctx, filePath, rootDir, state, &sum, &batch, batchSize, flush); err != nil {
				sum.Errors++
				log.Printf("error processing file %s: %v", filePath, err)
				// Continue with other files
			}
		}

		// Flush remaining batch
		if err := flush(); err != nil {
			sum.Errors++
			log.Printf("error flushing final batch: %v", err)
			return err
		}

		return nil
	})
	if err != nil {
		return sum, fmt.Errorf("failed to update index state: %w", err)
	}

	return sum, nil
}

// IngestURL ingests a single URL, updating the index and index state.
func (i *Ingestor) IngestURL(ctx context.Context, userURL string) (IngestSummary, error) {
	var sum IngestSummary

	// Get buildinfo for User-Agent
	userAgent := fmt.Sprintf("engramr/%s", getVersion())

	// Extract URL content
	urlContent, err := extract.ExtractURL(userURL, userAgent)
	if err != nil {
		return sum, err
	}

	// Ensure collection exists
	if err := i.Init(ctx); err != nil {
		return sum, err
	}

	// Get index state base directory
	baseDir := i.IndexStateBaseDir
	if baseDir == "" {
		baseDir = DefaultIndexStateBaseDir
	}
	indexStore := i.IndexState
	if indexStore == nil {
		indexStore = indexstate.NewFileStore()
	}

	// Use canonical URL as source path
	sourcePath := urlContent.CanonicalURL

	// Upsert in batches
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

	// Use transaction for read-modify-write cycle
	err = indexStore.Transaction(baseDir, func(state *indexstate.IndexState) error {
		// Check for deduplication
		oldEntry, exists := state.GetFile(sourcePath)
		if exists && oldEntry.ContentHash == urlContent.ContentHash {
			fmt.Printf("already ingested (no changes): %s\n", urlContent.CanonicalURL)
			return nil
		}

		// Chunk the markdown content (same as file ingestion)
		chunks := extract.ChunkMarkdown(sourcePath, urlContent.Content)
		if len(chunks) == 0 {
			// Remove URL from index state if it has no chunks
			state.SetFile(sourcePath, indexstate.FileEntry{
				Mtime:       urlContent.FetchedAt.Unix(),
				PointIDs:    []string{},
				ContentHash: urlContent.ContentHash,
			})
			return nil
		}

		sum.FilesIngested++

		// Process chunks: compute IDs, embed, build points
		newPointIDs, pointsToUpsert, err := i.processURLChunks(ctx, chunks, urlContent, &sum)
		if err != nil {
			return fmt.Errorf("processing chunks: %w", err)
		}

		// Delete stale points (old - new)
		deleteIDs := diffIds(oldEntry.PointIDs, newPointIDs)
		if len(deleteIDs) > 0 {
			if err := i.Store.DeletePoints(ctx, i.Collection, deleteIDs); err != nil {
				log.Printf("error deleting stale points for %s: %v", sourcePath, err)
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
				batch = append(batch, p)
				if len(batch) >= batchSize {
					if err := flush(); err != nil {
						return fmt.Errorf("flushing batch: %w", err)
					}
				}
			} else {
				sum.ChunksSkipped++
			}
		}

		// Flush remaining batch
		if err := flush(); err != nil {
			return fmt.Errorf("flushing final batch: %w", err)
		}

		// Update index state entry for this URL
		state.SetFile(sourcePath, indexstate.FileEntry{
			Mtime:       urlContent.FetchedAt.Unix(),
			PointIDs:    newPointIDs,
			ContentHash: urlContent.ContentHash,
		})

		return nil
	})
	if err != nil {
		return sum, fmt.Errorf("failed to update index state: %w", err)
	}

	return sum, nil
}

// processURLChunks processes URL chunks: embeds content, computes point IDs, and builds store points with URL metadata.
func (i *Ingestor) processURLChunks(ctx context.Context, chunks []extract.Chunk, urlContent *extract.URLContent, sum *IngestSummary) ([]string, []store.Point, error) {
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
			"source":        "url",
			"source_path":   c.SourcePath,
			"url":           urlContent.CanonicalURL, // Store original user URL if needed, but canonical is better
			"canonical_url": urlContent.CanonicalURL,
			"title":         sanitizeUTF8(urlContent.Title),
			"fetched_at":    urlContent.FetchedAt.Format("2006-01-02T15:04:05Z07:00"), // RFC3339
			"content_type":  urlContent.ContentType,
			"content_hash":  urlContent.ContentHash, // SHA256 of full extracted text
			"heading_path":  headingPathAny,
			"chunk_index":   c.ChunkIndex,
			"chunk_hash":    contentHash, // SHA1 of chunk content
			"modified_time": urlContent.FetchedAt.Unix(),
			"type":          "url",
			"content":       sanitizeUTF8(c.Content),
		}

		pointsToUpsert = append(pointsToUpsert, store.Point{
			ID:      pointID,
			Vector:  vec,
			Payload: payload,
		})
	}

	return newPointIDs, pointsToUpsert, nil
}

// getVersion returns the build version, defaulting to "dev" if not set.
func getVersion() string {
	if buildinfo.Version != "" && buildinfo.Version != "dev" {
		return buildinfo.Version
	}
	return "dev"
}

// processFile processes a single markdown file for ingestion.
func (i *Ingestor) processFile(
	ctx context.Context,
	filePath string,
	rootDir string,
	state *indexstate.IndexState,
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
		// Remove file from index state if it has no chunks
		state.SetFile(rel, indexstate.FileEntry{
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

	// Get old point IDs from index state
	oldEntry, _ := state.GetFile(rel)

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

	// Update index state entry for this file
	state.SetFile(rel, indexstate.FileEntry{
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
			"content":       sanitizeUTF8(c.Content),
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
		if err := filepath.WalkDir(absRoot, func(p string, d fs.DirEntry, walkErr error) error {
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
		}); err != nil {
			// Return empty list on walk error, caller can handle
			return nil
		}
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

// sanitizeUTF8 removes invalid UTF-8 sequences from a string, replacing them with the replacement character.
func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	// Convert to bytes and replace invalid UTF-8 sequences
	b := []byte(s)
	var result []byte
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 sequence, replace with replacement character
			result = append(result, 0xEF, 0xBF, 0xBD) // UTF-8 encoding of U+FFFD
			b = b[1:]
		} else {
			// Valid rune, append its UTF-8 encoding
			result = append(result, b[:size]...)
			b = b[size:]
		}
	}
	return string(result)
}
