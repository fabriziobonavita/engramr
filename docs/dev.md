# Dev notes

## Setup
```bash
go mod tidy
```

## Start dependencies
```bash
docker compose up -d
docker compose exec ollama ollama pull nomic-embed-text
```

## Build
```bash
make build
```

## Run
```bash
./engramr --help
```

## Test
```bash
make test
```

## Run integration tests
Integration tests verify end-to-end functionality (init -> ingest -> query) and require Qdrant and Ollama to be running.

```bash
# Start dependencies
docker compose up -d
docker compose exec ollama ollama pull nomic-embed-text

# Run integration tests
go test -tags=integration ./... -run Integration
```

Note: Integration tests are excluded from normal `go test ./...` runs and only execute when the `-tags=integration` flag is provided.
