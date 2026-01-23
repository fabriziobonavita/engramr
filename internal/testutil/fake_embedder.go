package testutil

import (
	"context"
)

// FakeEmbedder is a fake implementation of Embedder for testing.
// It can be used by both ingest and search packages since they share the same Embedder interface.
type FakeEmbedder struct {
	embeddings map[string][]float32
	err        error
}

// NewFakeEmbedder creates a new fake embedder.
func NewFakeEmbedder() *FakeEmbedder {
	return &FakeEmbedder{
		embeddings: make(map[string][]float32),
	}
}

// Embed implements the Embedder interface.
func (f *FakeEmbedder) Embed(ctx context.Context, input string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	if vec, ok := f.embeddings[input]; ok {
		return vec, nil
	}
	// Default: return a deterministic vector based on input length
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = float32(len(input) + i)
	}
	f.embeddings[input] = vec
	return vec, nil
}

// SetError sets an error to be returned on the next Embed call.
func (f *FakeEmbedder) SetError(err error) {
	f.err = err
}

// GetEmbeddedText returns all text that has been embedded (useful for search tests).
func (f *FakeEmbedder) GetEmbeddedText() []string {
	var texts []string
	for text := range f.embeddings {
		texts = append(texts, text)
	}
	return texts
}
