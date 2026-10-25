package cli

import (
	"bufio"
	"fmt"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/spf13/cobra"
)

func newSwitchCmd(core app.BranchSwitchService) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "switch <branch>", Short: "Review and switch to an existing local branch (clean workspace required)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		plan, err := core.PrepareBranchSwitch(cmd.Context(), ".", args[0])
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Trocar branch: %s -> %s\n", terminalText(plan.From), terminalText(plan.Target))
		if !yes {
			choice, err := readCommitChoice(bufio.NewReader(cmd.InOrStdin()), out, "Confirmar troca? [y/n]: ", []string{"y", "n"})
			if err != nil {
				return fmt.Errorf("confirmação interrompida; branch preservada (use --yes para scripts)")
			}
			if choice != "y" {
				fmt.Fprintln(out, "Troca cancelada.")
				return nil
			}
		}
		if err = core.SwitchBranch(cmd.Context(), ".", *plan); err != nil {
			return err
		}
		fmt.Fprintf(out, "Branch ativa: %s\n", terminalText(plan.Target))
		return nil
	}}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Switch to the reviewed local branch without interactive confirmation")
	return cmd
}
