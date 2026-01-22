# Engramr

Local-first omni-search for my notes (a deliberately scoped Go project).

Engramr is a personal project to build a fast, local-first vector-based search tool
over my own Markdown notes and drafts. The initial goal is "omni-search":
ingest -> search -> resurface relevant passages with citations.

Later (only if useful), it may evolve into a local-first cognitive helper
(summaries, synthesis, write-back), but that is explicitly not the v0 promise.

This is not a startup and not a generic RAG/chat demo.

## What it is (v0)
- Local-first semantic/vector search over Markdown files
- Qdrant as local vector store
- Ollama for local embeddings
- CLI-first and intentionally single-user
- Practical and reproducible (docker compose)

## What it is NOT (by design)
- No cloud services
- No accounts/auth/sync/collaboration
- No agent framework
- No "chat with your docs" product positioning
- No guarantees of stability (early project)

## Status
Early (v0). Public by design: correctness and learning over polish.

## Quickstart

### 1) Start dependencies
```bash
docker compose up -d
```

This starts:
- Qdrant at http://localhost:6333
- Ollama at http://localhost:11434

Pull an embedding model once:
```bash
docker compose exec ollama ollama pull nomic-embed-text
```

### 2) Build the CLI
```bash
go build -o engramr ./cmd/engramr
```

### 3) Initialize (creates collection if missing)
```bash
./engramr init
```

### 4) Ingest notes (recursively ingests *.md)
```bash
./engramr ingest ./notes
```

### 5) Query
```bash
./engramr query "Some anomaly" --top-k 10
```

Expected output (example):
1) notes/book/ch1.md :: Act I > Chapter 1
   score: 0.73
   ...snippet...

## Design notes (v0)
- Markdown-aware chunking (split by headings, then paragraphs)
- Deterministic chunk and point IDs for idempotent re-ingest
- Local manifest to track indexed state per file
- Everything runs locally

## Indexing semantics

Engramr uses deterministic point IDs and a local manifest file to ensure idempotent
ingest operations and clean index maintenance.
See docs/indexing.md for details.

## Troubleshooting

See docs/troubleshooting.md for common issues and solutions.

## Roadmap (non-binding)
- JSON output mode
- Optional summarization of retrieved passages
- Write-back / synthesis notes
- Desktop UI (maybe Tauri later)
- PDF ingestion

## License
MIT
