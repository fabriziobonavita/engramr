# Troubleshooting

Common issues and their solutions.

## Ollama Not Reachable

**Symptoms**: Errors like `ollama not reachable at http://localhost:11434` or connection timeouts.

**Solution**: Ensure the Ollama service is running:
```bash
docker compose up -d
```

Verify it's accessible:
```bash
curl http://localhost:11434/api/tags
```

## Model Not Pulled

**Symptoms**: Embedding requests fail with model not found errors.

**Solution**: Pull the embedding model:
```bash
docker compose exec ollama ollama pull nomic-embed-text
```

Verify the model is available:
```bash
docker compose exec ollama ollama list
```

## Qdrant Version Mismatch

**Symptoms**: gRPC errors, protocol mismatches, or collection creation failures after updating Qdrant.

**Solution**: 
1. Pin the Qdrant version in `docker-compose.yml` (e.g., `qdrant/qdrant:v1.16.3`)
2. Recreate the Qdrant container:
   ```bash
   docker compose down qdrant
   docker volume rm engramr_qdrant_data  # if you want a fresh start
   docker compose up -d qdrant
   ```
3. Re-initialize the collection: `./engramr init`

## Collection/Vector Size Mismatch

**Symptoms**: Errors about vector dimensions not matching, or collection configuration conflicts.

**Solution**: 
1. Delete the existing collection and recreate:
   ```bash
   # Via Qdrant UI: http://localhost:6333/dashboard
   # Or via API:
   curl -X DELETE http://localhost:6333/collections/engramr
   ./engramr init
   ```
2. Alternatively, use a fresh collection name by modifying the collection name in your configuration.

**Note**: Deleting a collection removes all indexed data. You'll need to re-run `ingest` after recreating the collection.
