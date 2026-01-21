package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
)

func TestUpsertSinglePoint(t *testing.T) {
	// Skip if Qdrant is not available (for CI/CD)
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	client := &QdrantClient{
		BaseURL: "http://localhost:6334",
	}

	// Create a test collection
	testCollection := "test_single_point_" + uuid.New().String()

	// Ensure collection exists with dimension 768 (matching your error message)
	err := client.EnsureCollection(ctx, testCollection, 4)
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	// Get the actual Qdrant client
	qdrantClient, err := client.getClient(ctx)
	if err != nil {
		t.Fatalf("failed to get Qdrant client: %v", err)
	}

	wait := true
	operationInfo, err := qdrantClient.Upsert(context.Background(), &qdrant.UpsertPoints{
		CollectionName: testCollection,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewIDNum(1),
				Vectors: qdrant.NewVectors(0.05, 0.61, 0.76, 0.74),
				Payload: qdrant.NewValueMap(map[string]any{"city": "London"}),
			},
			{
				Id:      qdrant.NewIDNum(2),
				Vectors: qdrant.NewVectors(0.19, 0.81, 0.75, 0.11),
				Payload: qdrant.NewValueMap(map[string]any{"age": 32}),
			},
			{
				Id:      qdrant.NewIDNum(3),
				Vectors: qdrant.NewVectors(0.36, 0.55, 0.47, 0.94),
				Payload: qdrant.NewValueMap(map[string]any{"vegan": true}),
			},
		},
		Wait: &wait,
	})

	if err != nil {
		t.Fatalf("failed to upsert point: %v", err)
		return
	}

	t.Logf("Successfully upserted point with vector dim: %s", operationInfo.Status.String())

}
