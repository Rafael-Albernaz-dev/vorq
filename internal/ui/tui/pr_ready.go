package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Mark a draft PR ready for review (VORQ-017, gap found with PR #1): the PRs
// screen offers "y pronto" on an open draft, a confirmation names the PR and
// repository, and the core re-checks the state before calling gh. No merge.

const modalPRReady modalID = "pr-ready"

func (m *Model) selectedDraftPR() (int, string, bool) {
	if m.screen != "prs" || m.prIndex < 0 || m.prIndex >= len(m.prs) {
		return 0, "", false
	}
	p := m.prs[m.prIndex]
	return p.Number, p.Title, p.Draft && (p.State == "OPEN" || p.State == "open")
}

func (m *Model) openPRReady() tea.Cmd {
	number, _, ok := m.selectedDraftPR()
	if !ok {
		m.notice = "Selecione um PR aberto em rascunho para marcar como pronto."
		return nil
	}
	m.readyPR = number
	m.modal, m.modalScroll = modalPRReady, 0
	return nil
}

func (m *Model) prReadyBody() string {
	title := ""
	for _, p := range m.prs {
		if p.Number == m.readyPR {
			title = p.Title
		}
	}
	repo := ""
	if m.studio != nil && m.studio.Repository != nil {
		repo = m.studio.Repository.Slug
	}
	return fmt.Sprintf("Marcar o PR #%d como pronto para revisão?\n\n%s\nRepositório: github.com/%s\n\nO GitHub passa a permitir o merge e pode notificar revisores. Nenhum merge é feito pelo vorq; para voltar a rascunho use gh pr ready %d --undo.", m.readyPR, title, repo, m.readyPR)
}

func (m *Model) markPRReady() tea.Cmd {
	if m.readyPR <= 0 || m.busy != "" || m.mutating {
		return nil
	}
	m.dialogID++
	id, core, dir, number, parent := m.dialogID, m.core, m.dir, m.readyPR, m.ctx
	m.busy, m.mutating = "Marcando PR como pronto…", true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 60*time.Second)
		defer cancel()
		err := core.MarkPullRequestReady(ctx, dir, number)
		return actionMsg{id: id, kind: "pr", notice: fmt.Sprintf("PR #%d pronto para revisão; o merge já pode ser feito no GitHub.", number), err: err}
	}
}
