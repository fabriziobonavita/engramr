package cmd

import (
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init command.
func NewInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize engramr (creates collection if missing)",
		Long: `Initialize engramr by creating the necessary Qdrant collection
if it doesn't already exist.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: not implemented
			cmd.Println("TODO: not implemented")
			return nil
		},
	}

	return cmd
}
