package indexstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// IndexState tracks which points belong to which files.
type IndexState struct {
	Version int                  `json:"version"`
	Files   map[string]FileEntry `json:"files"` // key: source_path
}

// FileEntry tracks metadata for a single file.
type FileEntry struct {
	Mtime    int64    `json:"mtime"`
	PointIDs []string `json:"point_ids"`
}

// Store is an interface for index state operations.
type Store interface {
	Load(baseDir string) (*IndexState, error)
	Save(baseDir string, state *IndexState) error
	// Transaction executes a read-modify-write cycle with a single lock.
	Transaction(baseDir string, fn func(*IndexState) error) error
}

// GetFile returns the FileEntry for the given source path, if it exists.
func (s *IndexState) GetFile(sourcePath string) (FileEntry, bool) {
	if s == nil || s.Files == nil {
		return FileEntry{}, false
	}
	entry, ok := s.Files[sourcePath]
	return entry, ok
}

// SetFile sets the FileEntry for the given source path.
func (s *IndexState) SetFile(sourcePath string, entry FileEntry) {
	if s == nil {
		return
	}
	if s.Files == nil {
		s.Files = make(map[string]FileEntry)
	}
	s.Files[sourcePath] = entry
}

// FileStore implements Store using the file system with cross-process locking.
type FileStore struct{}

// NewFileStore creates a new FileStore.
func NewFileStore() *FileStore {
	return &FileStore{}
}

// Load loads an index state from the given base directory.
// If the file doesn't exist, returns an empty index state.
// Acquires a lock for the read operation.
func (f *FileStore) Load(baseDir string) (*IndexState, error) {
	indexPath := filepath.Join(baseDir, "index.json")
	lockPath := filepath.Join(baseDir, "index.lock")

	// Acquire lock
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("index state is locked by another process")
	}
	defer lock.Unlock()

	// Read file
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &IndexState{
				Version: 1,
				Files:   make(map[string]FileEntry),
			}, nil
		}
		return nil, fmt.Errorf("failed to read index state: %w", err)
	}

	var state IndexState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse index state: %w", err)
	}

	if state.Files == nil {
		state.Files = make(map[string]FileEntry)
	}

	return &state, nil
}

// Save saves an index state to the given base directory using atomic write.
// Acquires a lock for the write operation.
func (f *FileStore) Save(baseDir string, state *IndexState) error {
	if state == nil {
		return fmt.Errorf("index state is nil")
	}

	indexPath := filepath.Join(baseDir, "index.json")
	lockPath := filepath.Join(baseDir, "index.lock")

	// Acquire lock
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("index state is locked by another process")
	}
	defer lock.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create index state directory: %w", err)
	}

	// Write to temp file first
	tmpPath := indexPath + ".tmp"
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index state: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp index state: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, indexPath); err != nil {
		os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("failed to rename temp index state: %w", err)
	}

	return nil
}

// Transaction executes a read-modify-write cycle with a single lock.
func (f *FileStore) Transaction(baseDir string, fn func(*IndexState) error) error {
	indexPath := filepath.Join(baseDir, "index.json")
	lockPath := filepath.Join(baseDir, "index.lock")

	// Acquire lock
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("index state is locked by another process")
	}
	defer lock.Unlock()

	// Load state
	var state *IndexState
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			state = &IndexState{
				Version: 1,
				Files:   make(map[string]FileEntry),
			}
		} else {
			return fmt.Errorf("failed to read index state: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &state); err != nil {
			return fmt.Errorf("failed to parse index state: %w", err)
		}
		if state.Files == nil {
			state.Files = make(map[string]FileEntry)
		}
	}

	// Execute transaction function
	if err := fn(state); err != nil {
		return err
	}

	// Save state
	if state == nil {
		return fmt.Errorf("index state is nil")
	}

	// Ensure directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create index state directory: %w", err)
	}

	// Write to temp file first
	tmpPath := indexPath + ".tmp"
	data, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index state: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp index state: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, indexPath); err != nil {
		os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("failed to rename temp index state: %w", err)
	}

	return nil
}

