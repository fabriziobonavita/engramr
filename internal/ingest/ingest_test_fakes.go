package ingest

import (
	"context"
	"fmt"

	"github.com/fabriziobonavita/engramr/internal/indexstate"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/fabriziobonavita/engramr/internal/testutil"
)

// newFakeEmbedder creates a fake embedder using the shared testutil implementation.
func newFakeEmbedder() *testutil.FakeEmbedder {
	return testutil.NewFakeEmbedder()
}

// fakeStore is a fake implementation of Store for testing.
type fakeStore struct {
	collections   map[string]int           // collection name -> vector size
	points        map[string][]store.Point // collection -> points
	deletedPoints map[string][]string      // collection -> deleted point IDs
	ensureErr     error
	upsertErr     error
	deleteErr     error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		collections:   make(map[string]int),
		points:        make(map[string][]store.Point),
		deletedPoints: make(map[string][]string),
	}
}

func (f *fakeStore) EnsureCollection(ctx context.Context, name string, vectorSize int) error {
	if f.ensureErr != nil {
		return f.ensureErr
	}
	f.collections[name] = vectorSize
	return nil
}

func (f *fakeStore) UpsertPoints(ctx context.Context, collection string, points []store.Point) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	if f.points[collection] == nil {
		f.points[collection] = make([]store.Point, 0)
	}
	// Simple upsert: replace points with same ID
	existing := make(map[string]int)
	for i, p := range f.points[collection] {
		existing[p.ID] = i
	}
	for _, p := range points {
		if idx, ok := existing[p.ID]; ok {
			f.points[collection][idx] = p
		} else {
			f.points[collection] = append(f.points[collection], p)
		}
	}
	return nil
}

func (f *fakeStore) DeletePoints(ctx context.Context, collection string, pointIDs []string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedPoints[collection] = append(f.deletedPoints[collection], pointIDs...)
	// Remove from points map
	if points, ok := f.points[collection]; ok {
		newPoints := make([]store.Point, 0, len(points))
		deletedSet := make(map[string]bool)
		for _, id := range pointIDs {
			deletedSet[id] = true
		}
		for _, p := range points {
			if !deletedSet[p.ID] {
				newPoints = append(newPoints, p)
			}
		}
		f.points[collection] = newPoints
	}
	return nil
}

func (f *fakeStore) getPoints(collection string) []store.Point {
	return f.points[collection]
}

func (f *fakeStore) getDeletedPoints(collection string) []string {
	return f.deletedPoints[collection]
}

// fakeIndexStateStore is a fake implementation of indexstate.Store for testing.
type fakeIndexStateStore struct {
	states  map[string]*indexstate.IndexState
	loadErr error
	saveErr error
}

func newFakeIndexStateStore() *fakeIndexStateStore {
	return &fakeIndexStateStore{
		states: make(map[string]*indexstate.IndexState),
	}
}

func (f *fakeIndexStateStore) Load(baseDir string) (*indexstate.IndexState, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if s, ok := f.states[baseDir]; ok {
		return s, nil
	}
	// Return empty index state if not found (matching FileStore behavior)
	state := &indexstate.IndexState{
		Version: 1,
		Files:   make(map[string]indexstate.FileEntry),
	}
	f.states[baseDir] = state
	return state, nil
}

func (f *fakeIndexStateStore) Save(baseDir string, state *indexstate.IndexState) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	if state == nil {
		return fmt.Errorf("index state is nil")
	}
	// Deep copy
	filesCopy := make(map[string]indexstate.FileEntry)
	for k, v := range state.Files {
		pointIDsCopy := make([]string, len(v.PointIDs))
		copy(pointIDsCopy, v.PointIDs)
		filesCopy[k] = indexstate.FileEntry{
			Mtime:       v.Mtime,
			PointIDs:    pointIDsCopy,
			ContentHash: v.ContentHash,
		}
	}
	f.states[baseDir] = &indexstate.IndexState{
		Version: state.Version,
		Files:   filesCopy,
	}
	return nil
}

func (f *fakeIndexStateStore) Transaction(baseDir string, fn func(*indexstate.IndexState) error) error {
	// Load state
	state, err := f.Load(baseDir)
	if err != nil {
		return err
	}

	// Execute transaction function
	if err := fn(state); err != nil {
		return err
	}

	// Save state
	return f.Save(baseDir, state)
}

func (f *fakeIndexStateStore) getState(baseDir string) *indexstate.IndexState {
	return f.states[baseDir]
}
