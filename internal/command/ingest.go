package command

import (
	"context"
	"fmt"
	"os"

	"github.com/fabriziobonavita/engramr/internal/embed"
	"github.com/fabriziobonavita/engramr/internal/ingest"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/spf13/cobra"
)

const (
	DefaultQdrantURL      = "http://localhost:6334"
	DefaultCollection     = "engramr_notes_v0"
	DefaultOllamaURL      = "http://localhost:11434"
	DefaultEmbeddingModel = "nomic-embed-text"
)

// NewIngestCmd creates the ingest command with subcommands.
func NewIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest content from files or URLs",
		Long: `Ingest content for semantic search.

Use 'ingest path' to ingest local Markdown files.
Use 'ingest url' to ingest web pages using readability-based extraction.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("usage changed: use engramr ingest path <path> or engramr ingest url <url>")
		},
	}

	cmd.AddCommand(newIngestPathCmd())
	cmd.AddCommand(newIngestURLCmd())

	return cmd
}

func newIngestPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <path>",
		Short: "Ingest Markdown files from a directory or file",
		Long: `Recursively ingest Markdown files from the specified path.
Files are chunked and embedded, then stored in Qdrant.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			ing := &ingest.Ingestor{
				Collection: DefaultCollection,
				Embedder: &embed.OllamaClient{
					BaseURL: DefaultOllamaURL,
					Model:   DefaultEmbeddingModel,
				},
				Store: &store.QdrantClient{
					BaseURL: DefaultQdrantURL,
				},
			}
			sum, err := ing.IngestPath(context.Background(), path)
			if err != nil {
				return err
			}

			cmd.Printf("files processed: %d\nchunks upserted: %d\nchunks deleted: %d\nchunks skipped: %d\n", sum.FilesIngested, sum.ChunksUpserted, sum.ChunksDeleted, sum.ChunksSkipped)
			if sum.Errors > 0 {
				cmd.Printf("errors: %d\n", sum.Errors)
			}
			return nil
		},
	}
}

func newIngestURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <url>",
		Short: "Ingest a web page via HTTP(S)",
		Long: `Ingest a web page using readability-based main content extraction.
The page is fetched, extracted, chunked, embedded, and stored in Qdrant.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			ing := &ingest.Ingestor{
				Collection: DefaultCollection,
				Embedder: &embed.OllamaClient{
					BaseURL: DefaultOllamaURL,
					Model:   DefaultEmbeddingModel,
				},
				Store: &store.QdrantClient{
					BaseURL: DefaultQdrantURL,
				},
			}
			sum, err := ing.IngestURL(context.Background(), url)
			if err != nil {
				return err
			}

			// Only print summary if we actually processed something (not deduplicated)
			if sum.FilesIngested > 0 {
				cmd.Printf("urls processed: %d\nchunks upserted: %d\nchunks deleted: %d\nchunks skipped: %d\n", sum.FilesIngested, sum.ChunksUpserted, sum.ChunksDeleted, sum.ChunksSkipped)
			}
			if sum.Errors > 0 {
				fmt.Fprintf(os.Stderr, "errors: %d\n", sum.Errors)
			}
			return nil
		},
	}
}
