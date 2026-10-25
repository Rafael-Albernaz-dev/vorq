package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/clipboard"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func printPlan(w io.Writer, p *domain.CommitPlan, diff bool) {
	fmt.Fprintf(w, "Plan %s · %d/%d completed\n", p.ID, len(p.Completed), len(p.Commits))
	for i, c := range p.Commits {
		fmt.Fprintf(w, "\n%d. %s\n%s\nReason: %s\n", i+1, terminalText(c.Subject), terminalText(c.Body), terminalText(c.Reason))
		for _, id := range c.Files {
			for _, f := range p.Snapshot.Files {
				if f.ID == id {
					fmt.Fprintf(w, "  %s %s\n", id, terminalText(f.Path))
					if diff {
						fmt.Fprintln(w, terminalText(f.Diff))
					}
				}
			}
		}
	}
	for _, f := range p.Snapshot.Files {
		if f.Excluded != "" {
			fmt.Fprintf(w, "Excluded %s: %s\n", terminalText(f.Path), terminalText(f.Excluded))
		}
	}
	for _, step := range p.Completed {
		fmt.Fprintf(w, "Created %s %s\n", step.Hash, terminalText(step.Subject))
	}
	if p.Error != "" {
		fmt.Fprintln(w, "Stopped:", terminalText(p.Error))
	}
}
func newAICmd(core app.AutomationService) *cobra.Command {
	root := &cobra.Command{Use: "ai", Short: "Review changes and organize approved whole-file commits"}
	for _, task := range []string{"review", "plan"} {
		task := task
		var provider, model string
		var asJSON bool
		cmd := &cobra.Command{Use: task, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			fmt.Fprintln(cmd.ErrOrStderr(), "Analyzing eligible working versions (including unstaged/new files); ignored and unsupported files excluded. No Git mutation before plan approval.")
			if task == "review" {
				review, err := core.ReviewDiffAI(ctx, ".", provider, model)
				if err != nil {
					return err
				}
				if asJSON {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(review)
				}
				fmt.Fprintln(cmd.OutOrStdout(), terminalText(review.Summary))
				for _, f := range review.Findings {
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s: %s\nEvidence: %s\n", f.Severity, f.FileID, terminalText(f.Message), terminalText(f.Evidence))
				}
				for _, f := range review.Snapshot.Files {
					if f.Excluded != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "Excluded %s: %s\n", terminalText(f.Path), terminalText(f.Excluded))
					}
				}
				return nil
			}
			plan, err := core.PlanCommitsAI(ctx, ".", provider, model)
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(plan)
			}
			printPlan(cmd.OutOrStdout(), plan, false)
			return nil
		}}
		cmd.Flags().StringVar(&provider, "provider", "codex", "codex or gemini (Antigravity)")
		cmd.Flags().StringVar(&model, "model", "", "Official CLI model ID")
		cmd.Flags().BoolVar(&asJSON, "json", false, "Print structured local result (includes snapshot content)")
		root.AddCommand(cmd)
	}
	var diff bool
	show := &cobra.Command{Use: "show", Short: "Inspect the saved plan; never executes it", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, e := core.LoadCommitPlan(cmd.Context(), ".")
		if e != nil {
			return e
		}
		printPlan(cmd.OutOrStdout(), p, diff)
		return nil
	}}
	show.Flags().BoolVar(&diff, "diff", false, "Show pinned diff of every group")
	root.AddCommand(show)
	var yes bool
	execute := &cobra.Command{Use: "execute", Short: "Review and execute/resume the saved plan; never pushes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		p, e := core.LoadCommitPlan(ctx, ".")
		if e != nil {
			return e
		}
		printPlan(cmd.OutOrStdout(), p, true)
		if !yes {
			choice, e := readCommitChoice(bufio.NewReader(cmd.InOrStdin()), cmd.OutOrStdout(), "Create remaining reviewed local commits? [y/n]: ", []string{"y", "n"})
			if e != nil {
				return fmt.Errorf("confirmation interrupted; nothing executed")
			}
			if choice != "y" {
				return nil
			}
		}
		result, e := core.ExecuteCommitPlan(ctx, ".", p)
		if result != nil {
			printPlan(cmd.OutOrStdout(), result, false)
		}
		return e
	}}
	execute.Flags().BoolVar(&yes, "yes", false, "Explicitly approve the saved plan without interactive confirmation")
	root.AddCommand(execute)
	var kind, fileID, subject, body string
	var group, target int
	edit := &cobra.Command{Use: "edit", Short: "Edit the saved plan without changing Git", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, e := core.LoadCommitPlan(cmd.Context(), ".")
		if e != nil {
			return e
		}
		p, e = core.EditCommitPlan(cmd.Context(), ".", p, domain.PlanEdit{Kind: kind, Group: group - 1, Target: target - 1, FileID: fileID, Subject: subject, Body: body})
		if e == nil {
			printPlan(cmd.OutOrStdout(), p, false)
		}
		return e
	}}
	edit.Flags().StringVar(&kind, "kind", "message", "message, move, exclude or order")
	edit.Flags().IntVar(&group, "group", 1, "Source group, one-based")
	edit.Flags().IntVar(&target, "target", 1, "Target group, one-based")
	edit.Flags().StringVar(&fileID, "file", "", "Snapshot file ID")
	edit.Flags().StringVar(&subject, "subject", "", "Commit subject")
	edit.Flags().StringVar(&body, "body", "", "Commit body")
	root.AddCommand(edit)
	return root
}
func newPathCmd(core app.InventoryService) *cobra.Command {
	var copy bool
	cmd := &cobra.Command{Use: "path <file>", Short: "Print the absolute path of a workspace file", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		p, e := core.AbsoluteFilePath(cmd.Context(), ".", args[0])
		if e != nil {
			return e
		}
		if copy {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if e = clipboard.Copy(ctx, p); e != nil {
				return e
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), terminalText(p))
		return nil
	}}
	cmd.Flags().BoolVar(&copy, "copy", false, "Write absolute path to the native clipboard")
	return cmd
}
