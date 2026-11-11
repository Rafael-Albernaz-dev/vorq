package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// Merge a PR from the PRs screen (VORQ-020 B): M opens a review with every
// blocker; 1/2/3 choose an allowed method; Enter merges pinned to the head.
// The result offers b (base + update) and d (delete the PR branch).

const modalPRMerge modalID = "pr-merge"

type prMergePlanMsg struct {
	id   uint64
	plan *domain.PRMergePlan
	err  error
}

func (m *Model) openPRMerge() tea.Cmd {
	if m.screen != "prs" || m.prIndex < 0 || m.prIndex >= len(m.prs) || m.busy != "" || m.mutating {
		m.notice = "Selecione um PR para mergear."
		return nil
	}
	m.dialogID++
	id, core, dir, number := m.dialogID, m.core, m.dir, m.prs[m.prIndex].Number
	m.modal, m.modalScroll = modalPRMerge, 0
	m.prMergePlan, m.prMergeMethod = nil, 0
	m.busy = "Consultando PR, checks e revisões no GitHub…"
	ctx, cancel := m.beginDialog(60 * time.Second)
	return func() tea.Msg {
		defer cancel()
		plan, err := core.PreparePRMerge(ctx, dir, number)
		return prMergePlanMsg{id, plan, err}
	}
}

func (m *Model) receivePRMergePlan(msg prMergePlanMsg) {
	if msg.id != m.dialogID {
		return
	}
	m.busy = ""
	if msg.err != nil {
		m.showError("Merge de PR", msg.err)
		return
	}
	m.prMergePlan = msg.plan
}

func (m *Model) prMergeKey(s string) tea.Cmd {
	p := m.prMergePlan
	if p == nil || m.busy != "" || m.mutating {
		return nil
	}
	for i := range p.Methods {
		if s == fmt.Sprint(i+1) {
			m.prMergeMethod = i
			return nil
		}
	}
	if s != "enter" {
		return nil
	}
	if len(p.Blockers) > 0 {
		m.notice = "Merge bloqueado: " + strings.Join(p.Blockers, "; ")
		return nil
	}
	m.dialogID++
	id, core, dir, plan, method, parent := m.dialogID, m.core, m.dir, *p, p.Methods[m.prMergeMethod], m.ctx
	m.busy, m.mutating = "Mergeando PR no GitHub…", true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 90*time.Second)
		defer cancel()
		if _, err := core.MergePullRequest(ctx, dir, plan, method); err != nil {
			return actionMsg{id: id, kind: "pr-merge", err: err}
		}
		return actionMsg{id: id, kind: "pr-merge", notice: fmt.Sprintf("PR #%d mergeado em %s (%s).\n\nLimpeza:\n  b  voltar para %s e atualizar\n  d  apagar a branch %s (local e remoto)", plan.Number, plan.Base, method, plan.Base, plan.Head)}
	}
}

func (m *Model) prMergeModal() (title, body, help, buttons string) {
	title, body = "MERGEAR PR", "Consultando PR, checks e revisões no GitHub…"
	help, buttons = "Esc voltar", "[Voltar]"
	p := m.prMergePlan
	if p == nil {
		return
	}
	title = fmt.Sprintf("MERGEAR PR #%d", p.Number)
	body = fmt.Sprintf("%s\n%s → %s · head %s\ngithub.com/%s\n\nChecks: %d/%d ok\n", safeText(p.Title), safeText(p.Head), safeText(p.Base), shortHash(p.HeadOID), safeText(p.Repository), p.ChecksPassed, p.ChecksTotal)
	if len(p.Blockers) > 0 {
		body += "\nNão dá para mergear agora:\n"
		for _, b := range p.Blockers {
			body += "  ! " + b + "\n"
		}
		body += "\nO vorq nunca força o merge (sem --admin)."
		return title, body, "Esc voltar", "[Voltar]"
	}
	body += "\nMétodo:\n"
	labels := map[string]string{"merge": "merge commit (preserva os commits)", "squash": "squash (um commit na base)", "rebase": "rebase (commits lineares na base)"}
	for i, method := range p.Methods {
		mark := "  "
		if i == m.prMergeMethod {
			mark = "› "
		}
		body += fmt.Sprintf("%s%d %s\n", mark, i+1, labels[method])
	}
	body += "\nO merge só acontece se o head ainda for " + shortHash(p.HeadOID) + " (commits novos invalidam a revisão). A branch não é apagada automaticamente."
	return title, body, "1-3 método · Enter mergear · Esc voltar", "[Confirmar] [Voltar]"
}

// postMergeKey handles the cleanup offers in the merge result.
func (m *Model) postMergeKey(s string) tea.Cmd {
	p := m.prMerged
	if p == nil || m.busy != "" || m.mutating {
		return nil
	}
	switch s {
	case "b":
		m.dialogID++
		id, core, dir, base, parent := m.dialogID, m.core, m.dir, p.Base, m.ctx
		m.busy, m.mutating = "Trocando para "+base+" e atualizando…", true
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
			defer cancel()
			plan, err := core.PrepareBranchSwitch(ctx, dir, base)
			if err == nil {
				err = core.SwitchBranch(ctx, dir, *plan)
			}
			if err != nil {
				return actionMsg{id: id, kind: "switch-branch", err: err}
			}
			target, err := core.GetPullTarget(ctx, dir)
			if err == nil {
				_, err = core.PullChanges(ctx, dir, *target)
			}
			if err != nil {
				return actionMsg{id: id, kind: "switch-branch", notice: "Em " + base + "; atualização não feita: " + err.Error()}
			}
			return actionMsg{id: id, kind: "switch-branch", notice: "Em " + base + ", atualizada com o merge."}
		}
	case "d":
		return m.reviewBranchDeleteOf(p.Head)
	}
	return nil
}
