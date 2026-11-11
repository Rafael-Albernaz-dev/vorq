package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/spf13/cobra"
)

func newSyncCmd(core app.SyncService) *cobra.Command {
	var yes, abort bool
	cmd := &cobra.Command{Use: "sync", Short: "Review, fetch and merge the remote default branch (e.g. main) into the current branch", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		if abort {
			if err := core.AbortMerge(cmd.Context(), "."); err != nil {
				return err
			}
			fmt.Fprintln(out, "Merge abortado; estado anterior restaurado.")
			return nil
		}
		target, err := core.GetSyncTarget(cmd.Context(), ".")
		if err != nil {
			return err
		}
		if target.MergeInProgress {
			printSyncConflicts(cmd, target.Conflicts)
			return fmt.Errorf("a merge is in progress; resolve and commit it, or run: vorq sync --abort")
		}
		if target.OnBase {
			return fmt.Errorf("already on %s; use: vorq pull", target.Base)
		}
		fmt.Fprintf(out, "Trazer: %s/%s -> Local: %s\n↓%d commit(s) conhecidos localmente (a busca pode trazer mais).\nMerge sem reescrever histórico; requer workspace limpo. Conflitos param o merge para você resolver.\n", terminalText(target.Remote), terminalText(target.Base), terminalText(target.Branch), target.Behind)
		if !yes {
			choice, err := readCommitChoice(bufio.NewReader(cmd.InOrStdin()), out, "Buscar e fazer merge? [y/n]: ", []string{"y", "n"})
			if err != nil {
				return fmt.Errorf("update confirmation interrupted; no fetch requested (use --yes in scripts)")
			}
			if choice != "y" {
				fmt.Fprintln(out, "Atualização cancelada.")
				return nil
			}
		}
		result, err := core.SyncFromBase(cmd.Context(), ".", *target)
		if err != nil {
			return err
		}
		switch {
		case result.UpToDate:
			fmt.Fprintf(out, "%s já contém %s/%s; nada a fazer.\n", terminalText(target.Branch), terminalText(target.Remote), terminalText(target.Base))
		case result.Stopped:
			printSyncConflicts(cmd, result.Conflicts)
			return fmt.Errorf("merge stopped for resolution; resolve, stage and commit, or run: vorq sync --abort")
		default:
			fmt.Fprintf(out, "%d commit(s) de %s/%s trazidos para %s.\n%s\n", result.Incoming, terminalText(target.Remote), terminalText(target.Base), terminalText(target.Branch), terminalText(strings.TrimSpace(result.Output)))
		}
		return nil
	}}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Fetch and merge the reviewed base without interactive confirmation")
	cmd.Flags().BoolVar(&abort, "abort", false, "Abort a merge stopped by conflicts and restore the previous state")
	return cmd
}

func printSyncConflicts(cmd *cobra.Command, paths []string) {
	out := cmd.OutOrStdout()
	if len(paths) == 0 {
		fmt.Fprintln(out, "Merge em andamento (sem arquivos em conflito; um hook pode ter interrompido).")
		return
	}
	fmt.Fprintf(out, "Conflitos (%d):\n", len(paths))
	for _, p := range paths {
		fmt.Fprintf(out, "  ! %s\n", terminalText(p))
	}
}
