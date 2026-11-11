package cli

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/spf13/cobra"
)

type prMergeCore interface {
	app.PRMergeService
	app.BranchSwitchService
	app.PullService
	app.BranchDeleteService
}

// newPRMergeCmd: `vorq pr merge <n>` reviews and merges a PR pinned to its
// head commit, then offers the cleanup (switch to the base + update, delete
// the PR branch locally and on the remote).
func newPRMergeCmd(core prMergeCore) *cobra.Command {
	var method string
	var yes bool
	c := &cobra.Command{Use: "merge <number>", Short: "Review and merge a PR (checks, review, conflicts); blocked PRs are never forced", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		n, e := strconv.Atoi(args[0])
		if e != nil || n <= 0 {
			return fmt.Errorf("invalid PR number %q", args[0])
		}
		out := c.OutOrStdout()
		plan, e := core.PreparePRMerge(c.Context(), ".", n)
		if e != nil {
			return e
		}
		printPRMergePlan(out, plan)
		if len(plan.Blockers) > 0 {
			return fmt.Errorf("PR #%d cannot be merged now; nothing was changed", n)
		}
		if method == "" {
			method = plan.Methods[0]
		}
		if !slices.Contains(plan.Methods, method) {
			return fmt.Errorf("merge method %q is not allowed here (allowed: %s)", method, strings.Join(plan.Methods, ", "))
		}
		reader := bufio.NewReader(c.InOrStdin())
		if !yes && !askYes(reader, out, fmt.Sprintf("Mergear o PR #%d com %s? [y/n]: ", n, method)) {
			fmt.Fprintln(out, "Merge cancelado; PR inalterado.")
			return nil
		}
		result, e := core.MergePullRequest(c.Context(), ".", *plan, method)
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "PR #%d mergeado em %s (%s).\n", n, terminalText(plan.Base), result.Method)
		if yes {
			fmt.Fprintf(out, "Limpeza: vorq switch %s && vorq pull · vorq branch -d %s\n", terminalText(plan.Base), terminalText(plan.Head))
			return nil
		}
		return prMergeCleanup(c, core, reader, plan)
	}}
	c.Flags().StringVar(&method, "method", "", "merge, squash or rebase (default: first allowed by the repository)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Merge without confirmation and skip the cleanup offers")
	return c
}

func askYes(reader *bufio.Reader, out io.Writer, prompt string) bool {
	choice, err := readCommitChoice(reader, out, prompt, []string{"y", "n"})
	return err == nil && choice == "y"
}

func printPRMergePlan(out io.Writer, p *domain.PRMergePlan) {
	fmt.Fprintf(out, "PR #%d · %s\n%s → %s · head %s · github.com/%s\n", p.Number, terminalText(p.Title), terminalText(p.Head), terminalText(p.Base), p.HeadOID[:min(12, len(p.HeadOID))], terminalText(p.Repository))
	fmt.Fprintf(out, "Checks: %d/%d ok · métodos permitidos: %s\n", p.ChecksPassed, p.ChecksTotal, strings.Join(p.Methods, ", "))
	for _, b := range p.Blockers {
		fmt.Fprintln(out, warnStyle.Render("  ! "+b))
	}
}

// prMergeCleanup offers each step separately; every step keeps its own review.
func prMergeCleanup(c *cobra.Command, core prMergeCore, reader *bufio.Reader, plan *domain.PRMergePlan) error {
	ctx, out := c.Context(), c.OutOrStdout()
	if askYes(reader, out, fmt.Sprintf("Voltar para %s e atualizar? [y/n]: ", plan.Base)) {
		sw, e := core.PrepareBranchSwitch(ctx, ".", plan.Base)
		if e == nil {
			e = core.SwitchBranch(ctx, ".", *sw)
		}
		if e != nil {
			fmt.Fprintln(out, warnStyle.Render("Troca não feita: "+e.Error()))
		} else if t, e := core.GetPullTarget(ctx, "."); e != nil {
			fmt.Fprintln(out, warnStyle.Render("Em "+plan.Base+"; atualização não feita: "+e.Error()))
		} else if _, e := core.PullChanges(ctx, ".", *t); e != nil {
			fmt.Fprintln(out, warnStyle.Render("Em "+plan.Base+"; atualização não feita: "+e.Error()))
		} else {
			fmt.Fprintf(out, "Em %s, atualizada.\n", terminalText(plan.Base))
		}
	}
	del, e := core.PrepareBranchDelete(ctx, ".", plan.Head)
	if e != nil {
		fmt.Fprintln(out, subtleStyle.Render("Branch do PR não apagada: "+e.Error()))
		return nil
	}
	printBranchDeletePlan(out, del)
	covered := app.MergedHeadCovers(*del, plan.HeadOID)
	if covered {
		fmt.Fprintln(out, "A ponta da branch é exatamente o commit mergeado no PR; nada se perde.")
	}
	if !askYes(reader, out, fmt.Sprintf("Apagar a branch %s (local e remoto)? [y/n]: ", plan.Head)) {
		return nil
	}
	if !covered && del.NeedsConfirm() {
		fmt.Fprintf(out, "Digite %s para apagar mesmo assim: ", domain.GuardOverrideWord)
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != domain.GuardOverrideWord {
			fmt.Fprintln(out, "Exclusão cancelada.")
			return nil
		}
	}
	result, e := core.DeleteBranch(ctx, ".", *del, covered || del.NeedsConfirm())
	if e != nil {
		return e
	}
	fmt.Fprintf(out, "Branch %s apagada (recuperar: git branch %s %s).\n", terminalText(del.Branch), terminalText(del.Branch), del.Hash)
	if result.RemoteDeleted {
		fmt.Fprintf(out, "Também apagada em %s.\n", terminalText(del.Remote))
	}
	return nil
}
