# Indexing Semantics

This document explains how Engramr indexes Markdown files, manages chunk IDs, and handles re-indexing.

## Markdown Chunking Rules

Engramr splits Markdown files into semantic chunks using a two-level approach:

1. **Section-level splitting**: The document is first split by headings (H1-H6). Each section under a heading becomes a unit of content.

2. **Paragraph-level chunking**: Within each section, content is further split into chunks:
   - Target size: ~1500 characters per chunk
   - Maximum size: 2500 characters per chunk
   - Overlap: 200 characters between adjacent chunks (to preserve context across boundaries)
   - Chunks are created by grouping paragraphs, respecting paragraph boundaries where possible
   - Very large paragraphs (>2500 chars) are hard-split if necessary

The heading path (e.g., `["Chapter 1", "Section A"]`) is preserved with each chunk for context.

## Deterministic Point IDs

Each chunk receives a **deterministic point ID** (UUID) that is stable across re-indexing operations. This enables idempotent ingestion: re-running `ingest` on the same files produces no duplicates.

The point ID is generated from a SHA1-based UUID using the following inputs:
- Source file path (relative to the ingest root)
- Heading path (joined with `/`)
- Chunk index within the section
- Content hash (SHA1 of the chunk content)

The same chunk (same file, heading context, position, and content) always produces the same point ID, regardless of when indexing occurs.

## Manifest File: `.engramr/index.json`

The manifest file tracks which point IDs belong to each indexed file. It stores:

- **File path** (relative to ingest root): The key for each entry
- **Modification time** (mtime): Unix timestamp of the file when last indexed
- **Point IDs**: Array of all point IDs currently associated with this file

The manifest enables efficient stale deletion: Engramr can determine which chunks to remove for a file without scanning the entire Qdrant collection.

## Reindex Semantics

Re-indexing is **idempotent** and handles file changes gracefully:

1. **Unchanged files**: If a file's content and mtime haven't changed, no work is done (chunks are skipped).

2. **Modified files**: 
   - New chunks are computed and embedded
   - Point IDs are compared between the manifest (old) and current indexing (new)
   - Stale chunks (present in old but not in new) are deleted from Qdrant
   - New or changed chunks are upserted

3. **Deleted files**: If a file no longer exists but has entries in the manifest, those point IDs can be cleaned up (currently requires manual manifest editing or re-initialization).

This approach ensures:
- No duplicate chunks on re-ingest
- Automatic cleanup of outdated chunks for edited files
- No expensive Qdrant scans to discover what to delete
