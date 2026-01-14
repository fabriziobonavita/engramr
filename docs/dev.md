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
