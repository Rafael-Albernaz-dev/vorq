package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var rightTabs = []string{"Commit", "Git", "PRs", "Worktrees"}
var rightTabIDs = []string{"commit", "git", "prs", "worktrees"}

type panelAction struct{ label, detail, key string }
type actionRow struct {
	text   string
	action int
}

func (m *Model) activeRightTab() string {
	if m.rightTab == "" {
		return "commit"
	}
	return m.rightTab
}
func (m *Model) selectRightTab(index int) {
	if index < 0 || index >= len(rightTabs) || m.mutating {
		return
	}
	m.rightTab = rightTabIDs[index]
	m.rightActionIndex = 0
	m.commitEditing = false
	m.focusPane = "commit"
}
func (m *Model) actionTabs(width int) string {
	widths := tabWidths(rightTabs, width)
	out := ""
	for i, label := range rightTabs {
		w := widths[i]
		bg, style := ColorPanel, dimStyle
		if m.activeRightTab() == rightTabIDs[i] {
			bg, style = ColorSelection, accentStyle
			label = "[" + label + "]"
		}
		out += surface(style.Render(lipgloss.PlaceHorizontal(w, lipgloss.Center, label)), w, bg)
	}
	return out
}
func (m *Model) panelActions() []panelAction {
	switch m.activeRightTab() {
	case "git":
		return []panelAction{{"Nova branch", "Criar e entrar em uma branch.", "b"}, {"Trocar branch", "Escolher uma branch local existente.", "switch-branch"}, {"Histórico de commits", "Ver branches, merges e diffs.", "H"}, {"Revisar push ↑", "Conferir destino antes de publicar.", "p"}, {"Atualizar branch ↓", "Buscar e aplicar somente fast-forward.", "pull"}}
	case "prs":
		return []panelAction{{"Listar PRs", "Consultar PRs, checks e revisões.", "P"}, {"Criar PR", "Preparar título, descrição e revisão.", "new-pr"}}
	case "worktrees":
		return []panelAction{{"Abrir worktrees", "Escolher um workspace existente.", "w"}}
	}
	return nil
}
func (m *Model) actionRows(width int) []actionRow {
	actions := m.panelActions()
	h := m.panelHeight()
	rows := []actionRow{{m.actionTabs(width), -1}, {surface(dimStyle.Render(" Escolha uma ação"), width, ColorPanel), -1}}
	if h >= len(actions)+4 {
		rows = append(rows, actionRow{surface("", width, ColorPanel), -1})
	}
	descriptions := h >= len(actions)*2+4
	for i, a := range actions {
		style := cyanStyle
		if i == m.rightActionIndex {
			style = accentStyle
		}
		rows = append(rows, actionRow{surface(style.Render(" ["+a.label+"]"), width, ColorRaised), i})
		if descriptions {
			rows = append(rows, actionRow{surface(dimStyle.Render(" "+a.detail), width, ColorPanel), -1})
		}
	}
	for len(rows) < h-1 {
		rows = append(rows, actionRow{surface("", width, ColorPanel), -1})
	}
	rows = append(rows, actionRow{surface(dimStyle.Render(" Clique · ↑/↓ escolher · Enter abrir"), width, ColorPanel), -1})
	return rows
}
func (m *Model) commitPane(width int) string {
	if m.activeRightTab() == "commit" {
		return m.commitEditorPane(width)
	}
	rows := m.actionRows(width)
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, r.text)
	}
	return fillPanel(lines, width, m.panelHeight(), ColorPanel)
}
func (m *Model) runPanelAction(index int) tea.Cmd {
	actions := m.panelActions()
	if index < 0 || index >= len(actions) || !m.canAct() {
		return nil
	}
	m.commitEditing = false
	switch actions[index].key {
	case "switch-branch":
		return m.inspectBranches()
	case "new-pr":
		return m.preparePR()
	case "pull":
		return m.inspectPull()
	default:
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(actions[index].key)})
	}
}
func (m *Model) actionPaneKey(key tea.KeyMsg) (bool, tea.Cmd) {
	if m.focusPane != "commit" || m.commitEditing {
		return false, nil
	}
	s := key.String()
	if s == "left" || s == "right" {
		index := 0
		for i, id := range rightTabIDs {
			if id == m.activeRightTab() {
				index = i
			}
		}
		delta := 1
		if s == "left" {
			delta = -1
		}
		m.selectRightTab((index + delta + len(rightTabs)) % len(rightTabs))
		return true, nil
	}
	if m.activeRightTab() == "commit" {
		return false, nil
	}
	actions := m.panelActions()
	switch s {
	case "up", "k":
		m.rightActionIndex = max(0, m.rightActionIndex-1)
		return true, nil
	case "down", "j", "tab":
		if len(actions) > 0 {
			m.rightActionIndex = (m.rightActionIndex + 1) % len(actions)
		}
		return true, nil
	case "enter", " ":
		return true, m.runPanelAction(m.rightActionIndex)
	}
	return false, nil
}
func (m *Model) contextArrowClick(x, y int) (bool, tea.Cmd) {
	contextY := 1
	if m.isWide {
		contextY = 2
	}
	if y != contextY || m.studio == nil {
		return false, nil
	}
	// Only the final context segment owns these hit areas. A workspace path
	// containing arrow text cannot become a transport control.
	branch := m.studio.Workspace.Branch
	a, b := m.studio.Changes.Ahead, m.studio.Changes.Behind
	right := m.contextBar(branch, a, b)
	if m.stale {
		right += " · DESATUALIZADO"
	}
	start := max(1, m.width-ansi.StringWidth(right)-3) + 2
	syncText := fmt.Sprintf("↑%d ↓%d", a, b)
	syncStart := start + ansi.StringWidth(displayName(branch)) + ansi.StringWidth(barSeparator())
	for _, label := range []string{fmt.Sprintf("↑%d", a), fmt.Sprintf("↓%d", b)} {
		if mouseLabelHit(syncText, label, x-syncStart) {
			m.commitEditing = false
			if label == fmt.Sprintf("↑%d", a) {
				return true, m.inspectPush()
			}
			return true, m.inspectPull()
		}
	}
	return false, nil
}
