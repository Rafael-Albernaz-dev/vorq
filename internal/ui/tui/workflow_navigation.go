package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) workflowKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "esc" {
		if m.busy != "" {
			m.closeWorkflow()
			return nil
		}
		if m.screen == "prs" && m.returnScreen == "history" {
			m.screen = "history"
			m.returnScreen = ""
			return m.selectHistory(0)
		}
		m.closeWorkflow()
		return nil
	}
	if s == "q" {
		return tea.Quit
	}
	if m.busy != "" {
		return nil
	}
	switch s {
	case "up", "k":
		if m.screen == "history" {
			return m.selectHistory(-1)
		}
		m.prIndex = max(0, m.prIndex-1)
		m.selectPR()
	case "down", "j":
		if m.screen == "history" {
			return m.selectHistory(1)
		}
		m.prIndex = min(max(0, len(m.prs)-1), m.prIndex+1)
		m.selectPR()
	case "pgdown":
		m.detailOffset += max(1, m.height-6)
	case "pgup":
		m.detailOffset = max(0, m.detailOffset-max(1, m.height-6))
	case "r":
		if m.screen == "history" {
			m.historyOptions.Skip = 0
			return m.loadHistory()
		}
		return m.loadPRs()
	case "/":
		m.openFilterForm()
	case "n":
		if m.screen == "history" && m.history != nil && m.history.HasMore {
			m.historyOptions.Skip = m.history.Skip + m.history.Count
			return m.loadHistory()
		}
		if m.screen == "prs" && m.prOptions.Limit < 500 {
			m.prOptions.Limit += 100
			return m.loadPRs()
		}
	case "[":
		if m.screen == "history" {
			m.historyOptions.Skip = max(0, m.historyOptions.Skip-m.historyOptions.Limit)
			return m.loadHistory()
		}
	case "P":
		return m.openPRs("")
	case "v":
		if m.screen == "history" && m.history != nil && len(m.history.Lines) > 0 {
			return m.openPRs(m.history.Lines[m.historyIndex].Hash)
		}
	case "c":
		if m.screen == "prs" {
			return m.preparePR()
		}
	case "e":
		if m.screen == "prs" {
			return m.inspectPR(true)
		}
	case "enter":
		if m.screen == "prs" {
			return m.inspectPR(false)
		}
	}
	return nil
}
func (m *Model) workflowView() string {
	rows := []string{}
	selected, offset := m.historyIndex, m.historyOffset
	title := "HISTÓRICO · grafo local"
	help := "↑/↓ commits · PgUp/PgDn detalhes · n próxima · [ anterior · / filtros · P PRs · v PRs do commit · r atualizar · Esc voltar"
	if m.screen == "history" && m.history != nil {
		for _, l := range m.history.Lines {
			line := l.Graph
			if l.Hash != "" {
				line += l.Hash[:8] + " " + l.Refs + " " + ansi.Truncate(l.Subject, 500, "…") + " · " + l.Author + " · " + l.Date
				for _, p := range m.associations[l.Hash] {
					line += fmt.Sprintf(" [PR #%d %s]", p.Number, p.State)
				}
			}
			rows = append(rows, safeText(line))
		}
		title += fmt.Sprintf(" · %d commits · offset %d · shallow: %t", m.history.Count, m.history.Skip, m.history.Shallow)
	}
	if m.screen == "prs" {
		selected, offset = m.prIndex, m.prOffset
		title = fmt.Sprintf("PULL REQUESTS · %s · %d/%d", m.prOptions.State, len(m.prs), m.prOptions.Limit)
		help = "↑/↓ selecionar · Enter detalhes · c criar · e editar · / filtros · n ampliar limite · PgUp/PgDn detalhes · Esc voltar"
		for _, p := range m.prs {
			rows = append(rows, fmt.Sprintf("#%d [%s] %s · %s → %s", p.Number, p.State, safeText(p.Title), safeText(p.Head), safeText(p.Base)))
		}
	}
	h := max(1, m.height-4)
	if selected < offset {
		offset = selected
	}
	if selected >= offset+h {
		offset = selected - h + 1
	}
	if m.screen == "history" {
		m.historyOffset = offset
	} else {
		m.prOffset = offset
	}
	left := max(20, m.width*45/100)
	right := max(1, m.width-left-1)
	if m.width < 90 {
		left = max(20, m.width/2)
		right = max(1, m.width-left-1)
	}
	details := strings.Split(ansi.Hardwrap(safeText(m.detailText), right, true), "\n")
	m.detailOffset = min(m.detailOffset, max(0, len(details)-h))
	lines := make([]string, h)
	for i := 0; i < h; i++ {
		l := ""
		if offset+i < len(rows) {
			l = rows[offset+i]
		}
		style := dimStyle
		if offset+i == selected {
			style = accentStyle.Background(ColorSelection)
		}
		d := ""
		if m.detailOffset+i < len(details) {
			d = details[m.detailOffset+i]
		}
		lines[i] = surface(style.Render(fit(l, left)), left, ColorPanel) + dimStyle.Render("│") + surface(fit(d, right), right, ColorBg)
	}
	status := m.notice
	if m.busy != "" {
		status = m.spinner() + " " + m.busy
	}
	base := strings.Join([]string{m.repositoryHeader(), surface(accentStyle.Render(fit(" "+title, m.width)), m.width, ColorPanel), strings.Join(lines, "\n"), surface(dimStyle.Render(fit(safeText(status), m.width)), m.width, ColorPanel), surface(fit(help, m.width), m.width, ColorRaised)}, "\n")
	if m.modal != "" {
		return overlay(base, m.modalView(), m.width, m.height)
	}
	return base
}
func (m *Model) workflowMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress {
		return nil
	}
	if msg.Y == 0 && msg.Button == tea.MouseButtonLeft {
		return m.openRepository()
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if msg.X >= m.width*45/100 {
			m.detailOffset += 3
			return nil
		}
		return m.workflowKey(tea.KeyMsg{Type: tea.KeyDown})
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if msg.X >= m.width*45/100 {
			m.detailOffset = max(0, m.detailOffset-3)
			return nil
		}
		return m.workflowKey(tea.KeyMsg{Type: tea.KeyUp})
	}
	if msg.Button != tea.MouseButtonLeft || m.busy != "" {
		return nil
	}
	if msg.Y == m.height-1 {
		line := strings.Split(m.workflowView(), "\n")
		footer := line[len(line)-1]
		for label, key := range map[string]string{"P PRs": "P", "v PRs do commit": "v", "c criar": "c", "e editar": "e", "n próxima": "n", "n ampliar limite": "n", "[ anterior": "[", "/ filtros": "/", "r atualizar": "r"} {
			if mouseLabelHit(footer, label, msg.X) {
				return m.workflowKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			}
		}
		if mouseLabelHit(footer, "Enter detalhes", msg.X) {
			return m.workflowKey(tea.KeyMsg{Type: tea.KeyEnter})
		}
		if mouseLabelHit(footer, "Esc voltar", msg.X) {
			return m.workflowKey(tea.KeyMsg{Type: tea.KeyEsc})
		}
	}
	if msg.X < m.width*45/100 && msg.Y >= 2 && msg.Y < m.height-3 {
		if m.screen == "history" && m.history != nil {
			target := min(len(m.history.Lines)-1, m.historyOffset+msg.Y-2)
			if target >= 0 {
				return m.selectHistory(target - m.historyIndex)
			}
		}
		if m.screen == "prs" && len(m.prs) > 0 {
			m.prIndex = min(len(m.prs)-1, m.prOffset+msg.Y-2)
			m.selectPR()
		}
	}
	return nil
}
