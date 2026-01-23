package command

import (
	"context"

	"github.com/fabriziobonavita/engramr/internal/embed"
	"github.com/fabriziobonavita/engramr/internal/ingest"
	"github.com/fabriziobonavita/engramr/internal/store"
	"github.com/spf13/cobra"
)

const (
	DefaultQdrantURL      = "http://localhost:6334"
	DefaultCollection    = "engramr_notes_v0"
	DefaultOllamaURL     = "http://localhost:11434"
	DefaultEmbeddingModel = "nomic-embed-text"
)

// NewIngestCmd creates the ingest command.
func NewIngestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest <path>",
		Short: "Ingest Markdown files from a directory",
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

	return cmd
}
