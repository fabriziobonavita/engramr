package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest tracks which points belong to which files.
type Manifest struct {
	Version int                  `json:"version"`
	Files   map[string]FileEntry `json:"files"` // key: source_path
}

// FileEntry tracks metadata for a single file.
type FileEntry struct {
	Mtime    int64    `json:"mtime"`
	PointIDs []string `json:"point_ids"`
}

// Load loads a manifest from the given path.
// If the file doesn't exist, returns an empty manifest.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{
				Version: 1,
				Files:   make(map[string]FileEntry),
			}, nil
		}
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if m.Files == nil {
		m.Files = make(map[string]FileEntry)
	}

	return &m, nil
}

// Save saves a manifest to the given path using atomic write (write temp then rename).
func Save(path string, m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	// Write to temp file first
	tmpPath := path + ".tmp"
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp manifest: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // best-effort cleanup
		return fmt.Errorf("failed to rename temp manifest: %w", err)
	}

	return nil
}

// GetFile returns the FileEntry for the given source path, if it exists.
func (m *Manifest) GetFile(sourcePath string) (FileEntry, bool) {
	if m == nil || m.Files == nil {
		return FileEntry{}, false
	}
	entry, ok := m.Files[sourcePath]
	return entry, ok
}

// SetFile sets the FileEntry for the given source path.
func (m *Manifest) SetFile(sourcePath string, entry FileEntry) {
	if m == nil {
		return
	}
	if m.Files == nil {
		m.Files = make(map[string]FileEntry)
	}
	m.Files[sourcePath] = entry
}
