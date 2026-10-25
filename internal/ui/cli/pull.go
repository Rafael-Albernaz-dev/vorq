package cli

import (
	"bufio"
	"fmt"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/spf13/cobra"
)

func newPullCmd(core app.PullService) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "pull", Short: "Review and fetch the upstream, then apply only a safe fast-forward", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		target, err := core.GetPullTarget(cmd.Context(), ".")
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Origem: %s/%s -> Local: %s\nFast-forward apenas; requer workspace limpo. A busca pode atualizar referências mesmo se a aplicação for recusada.\n", terminalText(target.Remote), terminalText(target.RemoteBranch), terminalText(target.Branch))
		if !yes {
			choice, err := readCommitChoice(bufio.NewReader(cmd.InOrStdin()), out, "Buscar e aplicar atualização? [y/n]: ", []string{"y", "n"})
			if err != nil {
				return fmt.Errorf("confirmação de atualização interrompida; nenhuma busca solicitada (use --yes para scripts)")
			}
			if choice != "y" {
				fmt.Fprintln(out, "Atualização cancelada.")
				return nil
			}
		}
		result, err := core.PullChanges(cmd.Context(), ".", *target)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, terminalText(result.Output))
		return nil
	}}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Fetch and fast-forward the reviewed target without interactive confirmation")
	return cmd
}
