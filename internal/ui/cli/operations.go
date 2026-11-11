package cli

import (
	"bufio"
	"fmt"
	"io"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/spf13/cobra"
)

// printOperation reports a finished or stopped cherry-pick/rebase/continue.
func printOperation(out io.Writer, r *domain.OperationResult) error {
	if !r.Stopped {
		fmt.Fprintf(out, "%s concluído.\n", r.Kind)
		if r.Output != "" {
			fmt.Fprintln(out, terminalText(r.Output))
		}
		return nil
	}
	printSyncConflictsTo(out, r.Conflicts)
	return fmt.Errorf("%s stopped for resolution; resolve, stage (git add) and run: vorq continue — or undo with: vorq abort", r.Kind)
}

func printSyncConflictsTo(out io.Writer, paths []string) {
	if len(paths) == 0 {
		fmt.Fprintln(out, "Operação parada sem arquivos em conflito (commit vazio ou hook do Git).")
		return
	}
	fmt.Fprintf(out, "Conflitos (%d):\n", len(paths))
	for _, p := range paths {
		fmt.Fprintf(out, "  ! %s\n", terminalText(p))
	}
}

func newCherryPickCmd(core app.OperationService) *cobra.Command {
	var yes bool
	c := &cobra.Command{Use: "cherry-pick <commit>", Short: "Review and apply one commit onto the current branch (conflicts stop for resolution)", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		out := c.OutOrStdout()
		plan, e := core.PrepareCherryPick(c.Context(), ".", args[0])
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Aplicar %s %s\nem %s (HEAD %s). Requer workspace limpo; o commit novo registra a origem (-x).\n", plan.Commit[:12], terminalText(plan.Subject), terminalText(plan.Branch), plan.HeadHash[:12])
		if !yes && !askYes(bufio.NewReader(c.InOrStdin()), out, "Aplicar? [y/n]: ") {
			fmt.Fprintln(out, "Cherry-pick cancelado.")
			return nil
		}
		r, e := core.CherryPick(c.Context(), ".", *plan)
		if e != nil {
			return e
		}
		return printOperation(out, r)
	}}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Apply without confirmation")
	return c
}

func newRebaseCmd(core app.OperationService) *cobra.Command {
	var yes bool
	c := &cobra.Command{Use: "rebase", Short: "Replay the current branch's unpublished commits onto the remote default branch (never needs force)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		out := c.OutOrStdout()
		plan, e := core.PrepareRebase(c.Context(), ".")
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Rebase de %s sobre %s/%s\n%d commit(s) locais reaplicados · ↓%d conhecidos localmente (a busca pode trazer mais).\nSó commits não publicados: nenhum force push será necessário.\n", terminalText(plan.Branch), terminalText(plan.Remote), terminalText(plan.Base), plan.Commits, plan.Behind)
		if !yes && !askYes(bufio.NewReader(c.InOrStdin()), out, "Buscar e fazer rebase? [y/n]: ") {
			fmt.Fprintln(out, "Rebase cancelado.")
			return nil
		}
		r, e := core.Rebase(c.Context(), ".", *plan)
		if e != nil {
			return e
		}
		return printOperation(out, r)
	}}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Fetch and rebase without confirmation")
	return c
}

func newContinueCmd(core app.OperationService) *cobra.Command {
	return &cobra.Command{Use: "continue", Short: "Continue a merge, cherry-pick or rebase stopped by conflicts (after resolving and staging)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		r, e := core.ContinueOperation(c.Context(), ".")
		if e != nil {
			return e
		}
		return printOperation(c.OutOrStdout(), r)
	}}
}

func newAbortCmd(core app.OperationService) *cobra.Command {
	var yes bool
	c := &cobra.Command{Use: "abort", Short: "Abort a stopped merge, cherry-pick or rebase and restore the previous state", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		out := c.OutOrStdout()
		state, e := core.GetOperationState(c.Context(), ".")
		if e != nil {
			return e
		}
		if state.Kind == "" {
			return fmt.Errorf("no Git operation in progress; nothing was changed")
		}
		fmt.Fprintf(out, "%s em andamento · %d arquivo(s) em conflito. Abortar descarta as resoluções feitas nele.\n", state.Kind, len(state.Conflicts))
		if !yes && !askYes(bufio.NewReader(c.InOrStdin()), out, "Abortar? [y/n]: ") {
			fmt.Fprintln(out, "Nada foi alterado.")
			return nil
		}
		kind, e := core.AbortOperation(c.Context(), ".")
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "%s abortado; estado anterior restaurado.\n", kind)
		return nil
	}}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Abort without confirmation")
	return c
}
