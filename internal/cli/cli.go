package cli

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/fabriziobonavita/engramr/internal/buildinfo"
	"github.com/fabriziobonavita/engramr/internal/command"
	"github.com/spf13/cobra"
)

// Run wires the CLI and executes it with the provided args, returning an exit code.
func Run(args []string) int {
	root := newRootCmd()
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	return 0
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "engramr",
		Short:         "Local-first semantic search for notes",
		Long:          "Engramr is a CLI tool for semantic search over local Markdown notes.\nIt uses Qdrant for vector storage and Ollama for embeddings.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		verbose, err := cmd.Flags().GetBool("verbose")
		if err != nil {
			return err
		}

		if verbose {
			log.SetOutput(os.Stderr)
			log.SetFlags(log.LstdFlags | log.Lmsgprefix)
			log.SetPrefix("DEBUG ")
		} else {
			log.SetOutput(io.Discard)
		}

		return nil
	}

	rootCmd.AddCommand(
		command.NewInitCmd(),
		command.NewIngestCmd(),
		command.NewQueryCmd(),
		newVersionCmd(),
		newCompletionCmd(rootCmd),
	)

	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show build version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "version: %s\ncommit: %s\ndate: %s\n",
				buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		},
	}
}

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate shell completion scripts",
		Args:                  cobra.ExactValidArgs(1),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
}
