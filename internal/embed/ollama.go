package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OllamaClient struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func (c *OllamaClient) Embed(ctx context.Context, input string) ([]float32, error) {
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 30 * time.Second}
	}
	base := strings.TrimRight(c.BaseURL, "/")

	reqBody := ollamaEmbeddingsRequest{
		Model:  c.Model,
		Prompt: input,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama not reachable at %s - run: docker compose up -d: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embeddings failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out ollamaEmbeddingsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	vec := make([]float32, len(out.Embedding))
	for i, v := range out.Embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}

type ollamaEmbeddingsRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbeddingsResponse struct {
	Embedding []float64 `json:"embedding"`
}
