package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/spf13/cobra"
)

type suggestionService interface {
	app.AISuggestionService
	app.CommitService
}

func newSuggestCmd(core suggestionService) *cobra.Command {
	var provider, model, commitType string
	cmd := &cobra.Command{
		Use: "suggest", Short: "Suggest an English commit message from staged changes (never creates a commit)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			if provider == "rules" {
				p, err := core.ProposeCommit(cmd.Context(), ".", commitType)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), terminalText(p.Message()))
				return nil
			}
			if provider != "codex" && provider != "gemini" && provider != "claude" {
				return fmt.Errorf("choose --provider codex, gemini, claude, or rules")
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Sending only the staged diff to %s using the official subscription CLI.\n", provider)
			p, err := core.SuggestCommitAI(ctx, ".", provider, model)
			if err != nil {
				return fmt.Errorf("%w\nRetry this command, change --provider, or use --provider rules; no fallback or commit was performed", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), terminalText(p.Proposal.Message()))
			return nil
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "codex", "codex, gemini (Antigravity), claude (Claude Code), or rules; AI sends staged code to the selected provider")
	cmd.Flags().StringVar(&model, "model", "", "Model ID from the selected provider's CLI catalog (empty uses the default)")
	cmd.Flags().StringVar(&commitType, "type", "feat", "Conventional Commit type for local rules")
	return cmd
}
