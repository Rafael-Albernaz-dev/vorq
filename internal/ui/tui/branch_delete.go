package tui

import (
	"context"
	"fmt"
	"time"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

type branchDeletePlanMsg struct {
	id   uint64
	plan *domain.BranchDeletePlan
	err  error
}

// reviewBranchDelete opens the deletion review for the selected branch.
func (m *Model) reviewBranchDelete() tea.Cmd {
	if len(m.localBranches) == 0 {
		return nil
	}
	return m.reviewBranchDeleteOf(m.localBranches[m.branchIndex].Name)
}

func (m *Model) reviewBranchDeleteOf(name string) tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = modalBranchDelete
	m.modalScroll = 0
	m.branchDeletePlan = nil
	m.branchDeleteInput = ""
	m.busy = "Revisando exclusão de branch…"
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		plan, err := core.PrepareBranchDelete(ctx, dir, name)
		return branchDeletePlanMsg{id, plan, err}
	}
}

func (m *Model) receiveBranchDeletePlan(msg branchDeletePlanMsg) {
	if msg.id != m.dialogID {
		return
	}
	m.busy = ""
	if msg.err != nil {
		m.showError("Apagar branch", msg.err)
		return
	}
	m.branchDeletePlan = msg.plan
}

// branchDeleteKey gives typed text precedence over shortcuts: q and letters
// go into the CONFIRMAR field when the deletion can lose commits.
func (m *Model) branchDeleteKey(key tea.KeyMsg) tea.Cmd {
	p := m.branchDeletePlan
	switch key.Type {
	case tea.KeyEsc:
		if m.mutating {
			return nil
		}
		m.dialogID++
		m.modal, m.branchDeleteInput = modalBranches, ""
		if m.prMerged != nil {
			m.modal = ""
		}
		return nil
	case tea.KeyEnter:
		if p == nil || !m.canAct() {
			return nil
		}
		confirmed := false
		if m.prMerged != nil && app.MergedHeadCovers(*p, m.prMerged.HeadOID) {
			confirmed = true // exactly the commit merged through the PR
		} else if p.NeedsConfirm() {
			if m.branchDeleteInput != domain.GuardOverrideWord {
				m.notice = "Digite exatamente " + domain.GuardOverrideWord + " (maiúsculas) ou Esc para voltar."
				return nil
			}
			confirmed = true
		}
		return m.deleteBranch(*p, confirmed)
	case tea.KeyBackspace:
		if r := []rune(m.branchDeleteInput); len(r) > 0 {
			m.branchDeleteInput = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		m.branchDeleteInput = ""
	case tea.KeyPgDown, tea.KeyDown:
		m.modalScroll++
	case tea.KeyPgUp, tea.KeyUp:
		m.modalScroll = max(0, m.modalScroll-1)
	case tea.KeyRunes:
		if p == nil || !m.branchDeleteNeedsTyping() {
			return nil
		}
		for _, r := range key.Runes {
			if unicode.IsPrint(r) && len(m.branchDeleteInput) < 32 {
				m.branchDeleteInput += string(r)
			}
		}
	}
	return nil
}

func (m *Model) deleteBranch(plan domain.BranchDeletePlan, confirmed bool) tea.Cmd {
	m.dialogID++
	id, core, dir, ctx := m.dialogID, m.core, m.dir, m.ctx
	m.busy = "Apagando branch…"
	m.mutating = true
	m.branchDeleteInput = ""
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		result, err := core.DeleteBranch(ctx, dir, plan, confirmed)
		if err != nil {
			return actionMsg{id: id, kind: "branch-delete", err: err}
		}
		notice := fmt.Sprintf("Branch %s apagada (era %s).\nRecuperar: git branch %s %s", plan.Branch, shortHash(plan.Hash), plan.Branch, plan.Hash)
		if result.RemoteDeleted {
			notice += fmt.Sprintf("\nTambém apagada em %s.", plan.Remote)
		}
		return actionMsg{id: id, kind: "branch-delete", notice: notice}
	}
}

func (m *Model) branchDeleteNeedsTyping() bool {
	p := m.branchDeletePlan
	return p != nil && p.NeedsConfirm() && (m.prMerged == nil || !app.MergedHeadCovers(*p, m.prMerged.HeadOID))
}

func shortHash(h string) string { return h[:min(12, len(h))] }

func (m *Model) branchDeleteModal() (title, body, help, buttons string) {
	title = "APAGAR BRANCH"
	body = "Revisando branch, base e remoto…"
	help, buttons = "Esc voltar", "[Voltar]"
	p := m.branchDeletePlan
	if p == nil {
		return
	}
	body = fmt.Sprintf("Branch: %s (%s)\nBase: %s\n\n", safeText(p.Branch), shortHash(p.Hash), safeText(p.Base))
	if p.Merged {
		body += "✓ Mergeada na base; nenhum commit se perde.\n"
	} else {
		body += fmt.Sprintf("! NÃO mergeada na base · %d commit(s) não publicados em nenhum remoto.\n", p.Unpublished)
	}
	if p.RemoteHash != "" {
		state := "mergeada"
		if !p.RemoteMerged {
			state = "NÃO mergeada"
		}
		body += fmt.Sprintf("Remoto: %s/%s também será apagada (%s). Recusado se alguém publicou commits novos nela.\n", safeText(p.Remote), safeText(p.Branch), state)
	} else {
		body += "Sem branch remota conhecida localmente; só a local é apagada.\n"
	}
	body += fmt.Sprintf("\nO hash fica no resultado para recuperar: git branch %s %s", safeText(p.Branch), shortHash(p.Hash))
	if m.prMerged != nil && app.MergedHeadCovers(*p, m.prMerged.HeadOID) {
		body += fmt.Sprintf("\n\nA ponta da branch é exatamente o commit mergeado no PR #%d; nada se perde.", m.prMerged.Number)
	}
	if m.branchDeleteNeedsTyping() {
		body += fmt.Sprintf("\n\nCommits podem se perder. Digite %s e Enter para apagar:\n> %s▏", domain.GuardOverrideWord, m.branchDeleteInput)
		return title, body, "Digite " + domain.GuardOverrideWord + " · Enter apagar · Ctrl+U limpar · Esc voltar", "[Apagar] [Voltar]"
	}
	return title, body, "Enter apagar · Esc voltar", "[Apagar] [Voltar]"
}
