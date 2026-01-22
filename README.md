# Engramr

Local-first omni-search for my notes (and a Go skills sandbox).

Engramr is a personal project to build a fast, local-first semantic search tool
over my own Markdown notes and drafts. The initial goal is "omni-search":
ingest -> search -> resurface relevant passages with citations.

Later (only if useful), it may evolve into a local-first cognitive helper
(summaries, synthesis, write-back), but that is not the v0 promise.

This is not a startup and not a generic "RAG demo".

## What it is (v0)
- Local-first semantic search over Markdown files
- Qdrant as local vector store
- Ollama for local embeddings
- CLI-first and intentionally single-user
- Practical and reproducible (docker compose)

## What it is NOT (by design)
- No cloud services
- No accounts/auth/sync/collaboration
- No agent framework
- No "chat with your docs" product positioning (yet)
- No guarantees of stability (early project)

## Status
Very early (v0). Public for transparency and learning, not polish.

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
- Stable chunk IDs to avoid duplication on re-ingest
- Everything runs locally

## Indexing semantics

Engramr uses deterministic point IDs and a local manifest file to ensure idempotent ingest operations and clean index maintenance.

**Deterministic point IDs**: Each chunk gets a stable UUID derived from its content and context using SHA1-based UUID generation. The same chunk (same source path, heading path, chunk index, and content hash) always produces the same point ID, enabling idempotent re-indexing.

**Manifest file**: A local manifest at `.engramr/index.json` tracks which point IDs belong to each indexed file. During ingest:
- The manifest records the point IDs for each file
- On re-ingest, old point IDs are compared with new ones
- Stale chunks (present in old but not in new) are automatically deleted from Qdrant
- This removes outdated chunks for edited files without scanning Qdrant

This approach ensures that:
- Re-running `ingest` on the same files is idempotent (no duplicates)
- Edited files automatically clean up their old chunks
- No Qdrant scans are needed to discover what to delete

## Roadmap (non-binding)
- JSON output mode
- Summarize top-K hits (minimal RAG)
- Write-back/synthesis notes
- Desktop UI (maybe Tauri later)
- PDF ingestion

## License
MIT
