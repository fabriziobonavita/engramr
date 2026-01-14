package app

import (
	"github.com/fabriziobonavita/engramr/internal/cmd"
	"github.com/spf13/cobra"
)

// NewApp creates and returns the root Cobra command.
func NewApp() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "engramr",
		Short: "Local-first semantic search for notes",
		Long: `Engramr is a CLI tool for semantic search over local Markdown notes.
It uses Qdrant for vector storage and Ollama for embeddings.`,
	}

	// Add --verbose flag
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")

	// Add subcommands
	rootCmd.AddCommand(cmd.NewInitCmd())
	rootCmd.AddCommand(cmd.NewIngestCmd())
	rootCmd.AddCommand(cmd.NewQueryCmd())

	return rootCmd
}
