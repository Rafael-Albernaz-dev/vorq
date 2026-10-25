package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(s))
}
func newBranchCmd(core app.WorkflowService) *cobra.Command {
	var o domain.BranchOptions
	c := &cobra.Command{Use: "branch <name>", Short: "Create and enter a local branch without stashing or discarding files", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		o.Name = args[0]
		if e := core.CreateBranch(c.Context(), ".", o); e != nil {
			return e
		}
		fmt.Fprintf(c.OutOrStdout(), "Branch criada: %s (entrar: %t)\n", terminalText(o.Name), !o.Stay)
		return nil
	}}
	c.Flags().StringVar(&o.Base, "base", "", "Base (default: current HEAD)")
	c.Flags().BoolVar(&o.Stay, "stay", false, "Create without switching")
	return c
}
func newTreeCmd(core app.WorkflowService) *cobra.Command {
	var o domain.HistoryOptions
	var commit string
	c := &cobra.Command{Use: "tree", Short: "Browse the local Git commit graph (no automatic fetch)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		if commit != "" {
			d, e := core.GetCommitDetail(c.Context(), ".", commit)
			if e != nil {
				return e
			}
			fmt.Fprintln(c.OutOrStdout(), terminalText(d.Text))
			return nil
		}
		p, e := core.GetHistory(c.Context(), ".", o)
		if e != nil {
			return e
		}
		out := c.OutOrStdout()
		for _, l := range p.Lines {
			if l.Hash == "" {
				fmt.Fprintln(out, terminalText(l.Graph))
				continue
			}
			fmt.Fprintf(out, "%s%s %s %s · %s · %s\n", terminalText(l.Graph), l.Hash[:8], terminalText(l.Refs), terminalText(l.Subject), terminalText(l.Author), terminalText(l.Date))
		}
		fmt.Fprintf(out, "%d commits · offset %d · shallow: %t · refs locais (sem fetch)\n", p.Count, p.Skip, p.Shallow)
		if p.HasMore {
			fmt.Fprintf(out, "Há mais commits: vorq tree --skip %d --limit %d\n", p.Skip+p.Count, o.Limit)
		}
		return nil
	}}
	c.Flags().IntVar(&o.Limit, "limit", 100, "Commits per page (1–500)")
	c.Flags().IntVar(&o.Skip, "skip", 0, "Skip commits")
	c.Flags().StringVar(&o.Ref, "ref", "", "Filter by branch/tag/revision")
	c.Flags().StringVar(&o.Search, "search", "", "Search commit messages")
	c.Flags().StringVar(&commit, "commit", "", "Show a full commit hash, files and diff")
	return c
}
func newPRCmd(core app.WorkflowService) *cobra.Command {
	root := &cobra.Command{Use: "pr", Short: "List, inspect, create and edit GitHub PRs using authenticated gh"}
	var o domain.PRListOptions
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		prs, e := core.ListPullRequests(c.Context(), ".", o)
		if e != nil {
			return e
		}
		for _, p := range prs {
			fmt.Fprintf(c.OutOrStdout(), "#%d [%s] %s · %s → %s · %s\n", p.Number, p.State, terminalText(p.Title), terminalText(p.Head), terminalText(p.Base), terminalText(p.URL))
		}
		if len(prs) == o.Limit {
			fmt.Fprintln(c.OutOrStdout(), "Limite atingido; refine --search ou aumente --limit (máximo 500).")
		}
		return nil
	}}
	list.Flags().StringVar(&o.State, "state", "open", "open, closed, merged or all")
	list.Flags().StringVar(&o.Search, "search", "", "GitHub search query")
	list.Flags().IntVar(&o.Limit, "limit", 100, "Maximum PRs (1–500)")
	view := &cobra.Command{Use: "view <number>", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		n, e := strconv.Atoi(args[0])
		if e != nil {
			return e
		}
		p, e := core.ViewPullRequest(c.Context(), ".", n)
		if e != nil {
			return e
		}
		raw, e := json.MarshalIndent(p, "", "  ")
		if e != nil {
			return e
		}
		fmt.Fprintln(c.OutOrStdout(), terminalText(string(raw)))
		return nil
	}}
	root.AddCommand(list, view, newPRSaveCmd(core, false), newPRSaveCmd(core, true))
	return root
}
func newPRSaveCmd(core app.WorkflowService, edit bool) *cobra.Command {
	var title, bodyFile, base string
	var labels, reviewers, remove []string
	var draft, yes bool
	use := "create"
	argsCheck := cobra.NoArgs
	if edit {
		use = "edit <number>"
		argsCheck = cobra.ExactArgs(1)
	}
	c := &cobra.Command{Use: use, Args: argsCheck, RunE: func(c *cobra.Command, args []string) error {
		var d domain.PRDraft
		if edit {
			n, e := strconv.Atoi(args[0])
			if e != nil {
				return e
			}
			pr, e := core.ViewPullRequest(c.Context(), ".", n)
			if e != nil {
				return e
			}
			link, e := core.GetRepositoryLink(c.Context(), ".")
			if e != nil {
				return e
			}
			d = domain.PRDraft{Number: n, Repository: link.Slug, Head: pr.Head, Base: pr.Base, Title: pr.Title, Body: pr.Body, Draft: pr.Draft}
		} else {
			p, e := core.PreparePullRequest(c.Context(), ".", base)
			if e != nil {
				return e
			}
			d = *p
			d.Draft = draft
		}
		if c.Flags().Changed("title") {
			d.Title = title
		}
		if c.Flags().Changed("base") {
			d.Base = base
		}
		if bodyFile != "" {
			raw, e := os.ReadFile(bodyFile)
			if e != nil {
				return e
			}
			d.Body = string(raw)
		}
		d.Labels, d.Reviewers, d.RemoveLabels = labels, reviewers, remove
		fmt.Fprintf(c.OutOrStdout(), "GitHub: %s · PR #%d · %s → %s · draft: %t\nTítulo: %s\n\n%s\n\nAdicionar labels: %s · reviewers: %s · remover labels: %s\n", d.Repository, d.Number, terminalText(d.Head), terminalText(d.Base), d.Draft, terminalText(d.Title), terminalText(d.Body), terminalText(strings.Join(labels, ", ")), terminalText(strings.Join(reviewers, ", ")), terminalText(strings.Join(remove, ", ")))
		if !yes {
			choice, e := readCommitChoice(bufio.NewReader(c.InOrStdin()), c.OutOrStdout(), "Enviar alteração de PR ao GitHub? [y/n]: ", []string{"y", "n"})
			if e != nil {
				return fmt.Errorf("confirmação de PR interrompida; nenhum envio solicitado (use --yes para scripts)")
			}
			if choice != "y" {
				fmt.Fprintln(c.OutOrStdout(), "PR cancelado.")
				return nil
			}
		}
		result, e := core.SavePullRequest(c.Context(), ".", d)
		if e != nil {
			return e
		}
		fmt.Fprintln(c.OutOrStdout(), terminalText(result))
		return nil
	}}
	c.Flags().StringVar(&title, "title", "", "PR title")
	c.Flags().StringVar(&bodyFile, "body-file", "", "Read PR body from UTF-8 file")
	c.Flags().StringVar(&base, "base", "", "Base branch (default: remote default branch)")
	c.Flags().StringSliceVar(&labels, "label", nil, "Labels to add")
	c.Flags().StringSliceVar(&reviewers, "reviewer", nil, "Reviewers to request")
	if edit {
		c.Flags().StringSliceVar(&remove, "remove-label", nil, "Labels to remove")
	} else {
		c.Flags().BoolVar(&draft, "draft", true, "Create as draft (use --draft=false for ready)")
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Send to displayed destination without interactive confirmation")
	return c
}
