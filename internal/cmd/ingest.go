package cmd

import (
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
			// TODO: not implemented
			cmd.Printf("TODO: not implemented (path: %s)\n", path)
			return nil
		},
	}

	return cmd
}
