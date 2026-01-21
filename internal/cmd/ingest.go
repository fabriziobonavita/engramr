package cmd

import (
	"context"

	"github.com/fabriziobonavita/engramr/internal/engine"
	"github.com/spf13/cobra"
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
			eng := engine.NewDefault()
			sum, err := eng.IngestPath(context.Background(), path)
			if err != nil {
				return err
			}

			cmd.Printf("files ingested: %d\nchunks upserted: %d\n", sum.FilesIngested, sum.ChunksUpserted)
			return nil
		},
	}

	return cmd
}
