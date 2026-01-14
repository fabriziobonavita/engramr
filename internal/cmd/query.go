package cmd

import (
	"github.com/spf13/cobra"
)

// NewQueryCmd creates the query command.
func NewQueryCmd() *cobra.Command {
	var topK int

	cmd := &cobra.Command{
		Use:   `query "<text>"`,
		Short: "Query the vector store for relevant passages",
		Long: `Search for semantically similar passages in ingested notes.
Returns the top-K most relevant results with scores and snippets.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			queryText := args[0]
			// TODO: not implemented
			cmd.Printf("TODO: not implemented (query: %q, top-k: %d)\n", queryText, topK)
			return nil
		},
	}

	cmd.Flags().IntVarP(&topK, "top-k", "k", 10, "Number of top results to return")

	return cmd
}
