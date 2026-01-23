package search

import (
	"context"
	"fmt"

	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/fabriziobonavita/engramr/internal/testutil"
)

// newFakeEmbedder creates a fake embedder using the shared testutil implementation.
func newFakeEmbedder() *testutil.FakeEmbedder {
	return testutil.NewFakeEmbedder()
}

// fakeStore is a fake implementation of Store for testing.
type fakeStore struct {
	collections   map[string]int // collection name -> vector size
	searchCalls   []searchCall
	searchResults map[string][]store.SearchResult // query vector hash -> results
	ensureErr     error
	searchErr     error
}

type searchCall struct {
	collection  string
	queryVector []float32
	topK        int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		collections:   make(map[string]int),
		searchCalls:   make([]searchCall, 0),
		searchResults: make(map[string][]store.SearchResult),
	}
}

func (f *fakeStore) EnsureCollection(ctx context.Context, name string, vectorSize int) error {
	if f.ensureErr != nil {
		return f.ensureErr
	}
	f.collections[name] = vectorSize
	return nil
}

func (f *fakeStore) Search(ctx context.Context, collection string, queryVector []float32, topK int) ([]store.SearchResult, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}

	// Record the call
	f.searchCalls = append(f.searchCalls, searchCall{
		collection:  collection,
		queryVector: queryVector,
		topK:        topK,
	})

	// Generate a hash of the query vector for lookup
	hash := fmt.Sprintf("%d-%d", len(queryVector), topK)
	if results, ok := f.searchResults[hash]; ok {
		return results, nil
	}

	// Return empty results by default
	return []store.SearchResult{}, nil
}

func (f *fakeStore) setSearchResults(hash string, results []store.SearchResult) {
	f.searchResults[hash] = results
}

func (f *fakeStore) getSearchCalls() []searchCall {
	return append([]searchCall(nil), f.searchCalls...)
}
