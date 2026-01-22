package store

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
)

type QdrantClient struct {
	BaseURL string
	client  *qdrant.Client
}

type Point struct {
	ID      string
	Vector  []float32
	Payload map[string]any
}

type SearchResult struct {
	ID      string
	Score   float64
	Payload map[string]any
}

func (c *QdrantClient) getClient(_ context.Context) (*qdrant.Client, error) {
	if c.client != nil {
		return c.client, nil
	}

	host, port, err := parseURL(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid qdrant URL %s: %w", c.BaseURL, err)
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host: host,
		Port: port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	c.client = client
	return client, nil
}

func parseURL(baseURL string) (string, int, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", 0, err
	}

	host := u.Hostname()
	if host == "" {
		host = "localhost"
	}

	// Qdrant Go client uses gRPC which runs on port 6334
	// If the URL specifies port 6333 (REST), use 6334 for gRPC instead
	port := 6334 // default gRPC port
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err == nil {
			if p == 6333 {
				// REST port specified, use gRPC port instead
				port = 6334
			} else {
				port = p
			}
		}
	}

	return host, port, nil
}

func (c *QdrantClient) EnsureCollection(ctx context.Context, name string, vectorSize int) error {
	client, err := c.getClient(ctx)
	if err != nil {
		return err
	}

	// Check if collection exists
	collections, err := client.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	if slices.Contains(collections, name) {
		return nil // collection exists
	}

	// Create collection
	err = client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(vectorSize),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

func (c *QdrantClient) DeleteCollection(ctx context.Context, name string) error {
	client, err := c.getClient(ctx)
	if err != nil {
		return err
	}

	err = client.DeleteCollection(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	return nil
}

func (c *QdrantClient) UpsertPoints(ctx context.Context, collection string, points []Point) error {
	if len(points) == 0 {
		return nil
	}

	client, err := c.getClient(ctx)
	if err != nil {
		return err
	}

	qdrantPoints := make([]*qdrant.PointStruct, 0, len(points))
	for _, p := range points {
		// Skip points with empty vectors
		if len(p.Vector) == 0 {
			continue
		}

		qdrantPoints = append(qdrantPoints, &qdrant.PointStruct{
			Id:      qdrant.NewID(p.ID),
			Vectors: qdrant.NewVectors(p.Vector...),
			Payload: qdrant.NewValueMap(p.Payload),
		})
	}

	if len(qdrantPoints) == 0 {
		return fmt.Errorf("no valid points to upsert (all had empty vectors)")
	}

	wait := true

	_, err = client.Upsert(context.Background(), &qdrant.UpsertPoints{
		CollectionName: collection,
		Points:         qdrantPoints,
		Wait:           &wait,
	})
	if err != nil {
		return fmt.Errorf("qdrant upsert failed: %w", err)
	}

	return nil
}

func (c *QdrantClient) DeletePoints(ctx context.Context, collection string, pointIDs []string) error {
	if len(pointIDs) == 0 {
		return nil
	}

	client, err := c.getClient(ctx)
	if err != nil {
		return err
	}

	// Convert string IDs to Qdrant IDs
	ids := make([]*qdrant.PointId, 0, len(pointIDs))
	for _, idStr := range pointIDs {
		ids = append(ids, qdrant.NewID(idStr))
	}

	wait := true
	_, err = client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points:         qdrant.NewPointsSelector(ids...),
		Wait:           &wait,
	})
	if err != nil {
		return fmt.Errorf("qdrant delete failed: %w", err)
	}

	return nil
}

func (c *QdrantClient) Search(ctx context.Context, collection string, queryVector []float32, topK int) ([]SearchResult, error) {
	client, err := c.getClient(ctx)
	if err != nil {
		return nil, err
	}

	limit := uint64(topK)
	searchResult, err := client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(queryVector...),
		Limit:          &limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant search failed: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResult))
	for _, hit := range searchResult {
		// Extract ID
		var idStr string
		if hit.Id != nil {
			if uuid := hit.Id.GetUuid(); uuid != "" {
				idStr = uuid
			} else if num := hit.Id.GetNum(); num != 0 {
				idStr = fmt.Sprintf("%d", num)
			} else {
				idStr = hit.Id.String()
			}
		}

		// Extract score (Score is float32 in ScoredPoint)
		score := float64(hit.Score)

		// Extract payload
		payload := make(map[string]any)
		if hit.Payload != nil {
			payload = convertPayload(hit.Payload)
		}

		results = append(results, SearchResult{
			ID:      idStr,
			Score:   score,
			Payload: payload,
		})
	}

	return results, nil
}

func convertPayload(payload map[string]*qdrant.Value) map[string]any {
	result := make(map[string]any)
	for k, v := range payload {
		if v == nil {
			continue
		}
		result[k] = convertValue(v)
	}
	return result
}

func convertValue(v *qdrant.Value) any {
	switch val := v.Kind.(type) {
	case *qdrant.Value_BoolValue:
		return val.BoolValue
	case *qdrant.Value_IntegerValue:
		return val.IntegerValue
	case *qdrant.Value_DoubleValue:
		return val.DoubleValue
	case *qdrant.Value_StringValue:
		return val.StringValue
	case *qdrant.Value_ListValue:
		list := make([]any, len(val.ListValue.Values))
		for i, item := range val.ListValue.Values {
			list[i] = convertValue(item)
		}
		return list
	case *qdrant.Value_StructValue:
		return convertPayload(val.StructValue.Fields)
	default:
		return nil
	}
}
