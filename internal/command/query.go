package command

import (
	"context"
	"strings"

	"github.com/fabriziobonavita/engramr/internal/embed"
	"github.com/fabriziobonavita/engramr/internal/search"
	"github.com/fabriziobonavita/engramr/internal/store"
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
			searcher := &search.Searcher{
				Collection: DefaultCollection,
				Embedder: &embed.OllamaClient{
					BaseURL: DefaultOllamaURL,
					Model:   DefaultEmbeddingModel,
				},
				Store: &store.QdrantClient{
					BaseURL: DefaultQdrantURL,
				},
			}
			hits, err := searcher.Query(context.Background(), queryText, topK)
			if err != nil {
				return err
			}

			for i, h := range hits {
				heading := strings.Join(h.HeadingPath, " > ")
				if heading == "" {
					heading = "(no heading)"
				}
				cmd.Printf("%d) %s :: %s\n", i+1, h.SourcePath, heading)
				cmd.Printf("   score: %.4f\n", h.Score)
				cmd.Printf("   snippet: %s\n\n", search.Snippet(h.Content, 200))
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&topK, "top-k", "k", 10, "Number of top results to return")

	return cmd
}
