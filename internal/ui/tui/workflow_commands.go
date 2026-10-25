package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) workflowContext() (context.Context, context.CancelFunc, uint64) {
	if m.workflowCancel != nil {
		m.workflowCancel()
	}
	m.workflowID++
	ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
	m.workflowCancel = cancel
	return ctx, cancel, m.workflowID
}
func (m *Model) closeWorkflow() {
	if m.workflowCancel != nil {
		m.workflowCancel()
	}
	if m.detailCancel != nil {
		m.detailCancel()
	}
	m.workflowID++
	m.detailID++
	m.busy = ""
	m.screen = ""
	m.detailText = ""
	m.modal = ""
	m.returnScreen = ""
}
func (m *Model) openHistory() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.screen = "history"
	m.history = nil
	m.historyOptions = domain.HistoryOptions{Limit: 100}
	m.historyIndex = 0
	m.historyOffset = 0
	m.detailText = ""
	return m.loadHistory()
}
func (m *Model) loadHistory() tea.Cmd {
	ctx, cancel, id := m.workflowContext()
	core, dir, o := m.core, m.dir, m.historyOptions
	m.busy = "Lendo histórico local…"
	return func() tea.Msg {
		defer cancel()
		p, e := core.GetHistory(ctx, dir, o)
		return workflowMsg{id: id, kind: "history", dir: dir, page: p, err: e}
	}
}
func (m *Model) openPRs(search string) tea.Cmd {
	if m.busy != "" || m.mutating {
		return nil
	}
	if m.screen == "history" {
		m.returnScreen = "history"
	} else {
		m.returnScreen = ""
	}
	m.screen = "prs"
	m.prs = nil
	m.prIndex = 0
	m.prOffset = 0
	m.detailText = ""
	m.prOptions = domain.PRListOptions{State: "all", Limit: 100, Search: search}
	return m.loadPRs()
}
func (m *Model) loadPRs() tea.Cmd {
	ctx, cancel, id := m.workflowContext()
	core, dir, o := m.core, m.dir, m.prOptions
	m.busy = "Consultando PRs no GitHub…"
	return func() tea.Msg {
		defer cancel()
		prs, e := core.ListPullRequests(ctx, dir, o)
		return workflowMsg{id: id, kind: "prs", dir: dir, prs: prs, err: e}
	}
}
func (m *Model) updateWorkflow(msg workflowMsg) tea.Cmd {
	if msg.id != m.workflowID || msg.dir != m.dir {
		return nil
	}
	m.busy = ""
	if msg.err != nil {
		m.showError("Fluxo Git", msg.err)
		return nil
	}
	switch msg.kind {
	case "history":
		if m.history != nil && m.historyOptions.Skip > 0 && m.history.Snapshot != msg.page.Snapshot {
			m.historyOptions.Skip = 0
			m.notice = "Referências mudaram; histórico reiniciado para evitar lacunas."
			m.history = nil
			return m.loadHistory()
		}
		m.history = msg.page
		m.historyIndex = 0
		m.historyOffset = 0
		m.detailOffset = 0
		m.notice = "Histórico local · referências remotas podem estar desatualizadas (sem fetch)."
		return m.selectHistory(0)
	case "prs":
		m.prs = msg.prs
		m.prIndex = 0
		m.prOffset = 0
		m.detailOffset = 0
		if m.associations == nil {
			m.associations = map[string][]domain.PullRequest{}
		}
		for _, pr := range msg.prs {
			if pr.HeadOID != "" {
				m.associations[pr.HeadOID] = appendPR(m.associations[pr.HeadOID], pr)
			}
			if pr.MergeCommit != nil && pr.MergeCommit.OID != "" {
				hash := pr.MergeCommit.OID
				m.associations[hash] = appendPR(m.associations[hash], pr)
			}
		}
		m.notice = fmt.Sprintf("%d PRs · consulta explícita ao GitHub · limite %d; use / para filtrar.", len(m.prs), m.prOptions.Limit)
		m.selectPR()
	case "prepare":
		m.prDraft = *msg.draft
		m.setupPRForm()
	case "edit":
		pr := msg.pr
		m.prDraft = domain.PRDraft{Number: pr.Number, Title: pr.Title, Body: pr.Body, Head: pr.Head, Base: pr.Base, Draft: pr.Draft}
		if m.studio != nil && m.studio.Repository != nil {
			m.prDraft.Repository = m.studio.Repository.Slug
		}
		m.setupPRForm()
	}
	return nil
}
func appendPR(prs []domain.PullRequest, pr domain.PullRequest) []domain.PullRequest {
	for i, p := range prs {
		if p.Number == pr.Number {
			prs[i] = pr
			return prs
		}
	}
	return append(prs, pr)
}
func (m *Model) updateWorkflowDetail(msg workflowDetailMsg) tea.Cmd {
	if msg.id == 0 {
		if msg.err != nil {
			m.notice = "Não foi possível abrir o navegador: " + msg.err.Error()
		}
		return nil
	}
	if msg.id != m.detailID || m.screen == "" {
		return nil
	}
	if msg.err != nil {
		m.detailText = msg.err.Error()
	} else {
		m.detailText = msg.text
	}
	m.detailOffset = 0
	return nil
}
func (m *Model) selectHistory(delta int) tea.Cmd {
	if m.history == nil || len(m.history.Lines) == 0 {
		m.detailText = "Nenhum commit neste filtro."
		return nil
	}
	i := min(max(0, m.historyIndex+delta), len(m.history.Lines)-1)
	step := 1
	if delta < 0 {
		step = -1
	}
	for i >= 0 && i < len(m.history.Lines) && m.history.Lines[i].Hash == "" {
		i += step
	}
	if i < 0 || i >= len(m.history.Lines) {
		return nil
	}
	m.historyIndex = i
	if m.detailCancel != nil {
		m.detailCancel()
	}
	m.detailID++
	id, hash, core, dir := m.detailID, m.history.Lines[i].Hash, m.core, m.dir
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	m.detailCancel = cancel
	m.detailText = "Carregando commit…"
	m.detailOffset = 0
	return func() tea.Msg {
		defer cancel()
		d, e := core.GetCommitDetail(ctx, dir, hash)
		text := ""
		if d != nil {
			text = d.Text
		}
		return workflowDetailMsg{id: id, text: text, err: e}
	}
}
func (m *Model) selectPR() {
	m.detailID++
	if m.detailCancel != nil {
		m.detailCancel()
	}
	m.detailOffset = 0
	if len(m.prs) == 0 {
		m.detailText = "Nenhum PR neste filtro."
		return
	}
	p := m.prs[m.prIndex]
	m.detailText = fmt.Sprintf("#%d · %s\n%s → %s\n%s · draft: %t\n%s\n\nEnter consulta descrição, checks, revisões e comentários.", p.Number, p.Title, p.Head, p.Base, p.State, p.Draft, p.URL)
}
func prText(p *domain.PullRequest) string {
	text := fmt.Sprintf("#%d %s\n%s → %s · %s · draft: %t\n%s\nRevisão: %s\n\n%s\n\nCHECKS\n", p.Number, p.Title, p.Head, p.Base, p.State, p.Draft, p.URL, p.ReviewDecision, p.Body)
	for _, c := range p.Checks {
		text += fmt.Sprintf("%s%s: %s %s %s\n", c.Name, c.Context, c.Status, c.Conclusion, c.State)
	}
	text += "\nREVISÕES\n"
	for _, r := range p.Reviews {
		text += fmt.Sprintf("%s · %s\n%s\n", r.Author.Login, r.State, r.Body)
	}
	text += "\nCOMENTÁRIOS\n"
	for _, c := range p.Comments {
		text += fmt.Sprintf("%s: %s\n", c.Author.Login, c.Body)
	}
	return text
}
func (m *Model) inspectPR(edit bool) tea.Cmd {
	if len(m.prs) == 0 {
		return nil
	}
	n, core, dir := m.prs[m.prIndex].Number, m.core, m.dir
	if edit {
		ctx, cancel, id := m.workflowContext()
		m.busy = "Lendo PR para edição…"
		m.modal = "workflow"
		m.fields = nil
		return func() tea.Msg {
			defer cancel()
			p, e := core.ViewPullRequest(ctx, dir, n)
			return workflowMsg{id: id, kind: "edit", dir: dir, pr: p, err: e}
		}
	}
	if m.detailCancel != nil {
		m.detailCancel()
	}
	m.detailID++
	id := m.detailID
	ctx, cancel := context.WithTimeout(m.ctx, 45*time.Second)
	m.detailCancel = cancel
	m.detailText = "Consultando detalhes do PR…"
	return func() tea.Msg {
		defer cancel()
		p, e := core.ViewPullRequest(ctx, dir, n)
		text := ""
		if p != nil {
			text = prText(p)
		}
		return workflowDetailMsg{id: id, text: text, err: e}
	}
}
