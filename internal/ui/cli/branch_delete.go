package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/spf13/cobra"
)

// branchDelete is `vorq branch -d <name>`: review, confirm, delete local and
// remote. Unmerged commits need --allow-unmerged plus CONFIRMAR (or --yes).
func branchDelete(c *cobra.Command, core app.BranchDeleteService, name string, allow, yes bool) error {
	out := c.OutOrStdout()
	plan, err := core.PrepareBranchDelete(c.Context(), ".", name)
	if err != nil {
		return err
	}
	printBranchDeletePlan(out, plan)
	reader := bufio.NewReader(c.InOrStdin())
	confirmed := false
	switch {
	case plan.NeedsConfirm() && !allow:
		fmt.Fprintln(out, subtleStyle.Render("Para apagar mesmo assim: --allow-unmerged (confirmação digitada)."))
		return fmt.Errorf("branch %s has commits not merged into %s; nothing was deleted", plan.Branch, plan.Base)
	case plan.NeedsConfirm() && !yes:
		fmt.Fprintf(out, "Digite %s para apagar mesmo assim (outra resposta cancela): ", domain.GuardOverrideWord)
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != domain.GuardOverrideWord {
			fmt.Fprintln(out, "Exclusão cancelada. Nada foi apagado.")
			return nil
		}
		confirmed = true
	case plan.NeedsConfirm():
		confirmed = true
	case !yes:
		choice, err := readCommitChoice(reader, out, "Apagar? [y/n]: ", []string{"y", "n"})
		if err != nil {
			return fmt.Errorf("deletion confirmation interrupted; nothing deleted (use --yes in scripts)")
		}
		if choice != "y" {
			fmt.Fprintln(out, "Exclusão cancelada.")
			return nil
		}
	}
	result, err := core.DeleteBranch(c.Context(), ".", *plan, confirmed)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Branch %s apagada (era %s; recuperar: git branch %s %s).\n", terminalText(plan.Branch), plan.Hash[:min(12, len(plan.Hash))], terminalText(plan.Branch), plan.Hash)
	if result.RemoteDeleted {
		fmt.Fprintf(out, "Também apagada em %s.\n", terminalText(plan.Remote))
	}
	return nil
}

func printBranchDeletePlan(out io.Writer, p *domain.BranchDeletePlan) {
	fmt.Fprintf(out, "Apagar branch: %s (%s)\n", terminalText(p.Branch), p.Hash[:min(12, len(p.Hash))])
	if p.Merged {
		fmt.Fprintf(out, "Mergeada em %s.\n", terminalText(p.Base))
	} else {
		fmt.Fprintln(out, warnStyle.Render(fmt.Sprintf("NÃO mergeada em %s; %d commit(s) não publicados.", p.Base, p.Unpublished)))
	}
	if p.RemoteHash != "" {
		state := "mergeada"
		if !p.RemoteMerged {
			state = "NÃO mergeada"
		}
		fmt.Fprintf(out, "Remoto: %s/%s também será apagada (%s; recusado se tiver commits novos).\n", terminalText(p.Remote), terminalText(p.Branch), state)
	} else {
		fmt.Fprintln(out, "Sem branch remota conhecida localmente (nada a apagar no remoto).")
	}
}
