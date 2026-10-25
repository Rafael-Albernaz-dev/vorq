package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKey(key tea.KeyMsg) tea.Cmd {
	s := key.String()
	if s == "ctrl+c" {
		return tea.Quit
	}
	if m.modal == "workflow" {
		return m.workflowFormKey(key)
	}
	if m.modal != "" {
		return m.handleModalKey(key)
	}

	if m.screen != "" {
		return m.workflowKey(key)
	}

	// Editing wins over every printable global shortcut, including q, a, e and s.
	if m.focusPane == "commit" && m.commitEditing {
		switch s {
		case "esc":
			m.commitEditing = false
			m.commitField = "actions"
		case "shift+tab":
			m.cycleFocus()
		case "tab":
			m.cycleCommitField()
		case "ctrl+s", "alt+enter":
			if !m.mutating {
				return m.review()
			}
		case "ctrl+g":
			if m.busy == "" {
				m.openSuggestion()
			}
		default:
			if !m.mutating {
				m.editKey(key)
			}
		}
		return nil
	}
	if handled, cmd := m.actionPaneKey(key); handled {
		return cmd
	}
	if s == "q" {
		return tea.Quit
	}
	if s == "esc" {
		m.focusPane = "tree"
		m.commitEditing = false
		return nil
	}
	if s == "shift+tab" {
		m.cycleFocus()
		return nil
	}
	if s == "?" {
		m.modal = "help"
		m.modalScroll = 0
		return nil
	}
	if s == "t" {
		m.modal = "appearance"
		m.modalScroll = 0
		return nil
	}
	if s == "i" {
		m.cycleIcons()
		return nil
	}
	if s == "a" || s == "e" {
		tab := "alteracoes"
		if s == "e" {
			tab = "repositorio"
		}
		m.switchTree(tab)
		m.focusPane = "tree"
		return m.loadSelected()
	}
	if s == "1" {
		m.setViewerTab("trabalho")
		return nil
	}
	if s == "2" {
		m.setViewerTab("stage")
		return nil
	}
	if s == "tab" {
		if m.focusPane == "commit" {
			m.cycleCommitField()
		} else if m.viewerTab == "arquivo" {
			tab := m.lastDiffTab
			if tab == "" {
				tab = "trabalho"
			}
			m.setViewerTab(tab)
		} else {
			m.lastDiffTab = m.viewerTab
			m.setViewerTab("arquivo")
		}
		return nil
	}
	if m.busy != "" {
		return nil
	}
	if s == "r" && !m.loading {
		return m.refresh()
	}
	if s == "b" {
		m.openBranchForm()
		return nil
	}
	if s == "H" {
		return m.openHistory()
	}
	if s == "P" {
		return m.openPRs("")
	}
	if s == "o" {
		return m.openRepository()
	}
	if s == "w" {
		return m.inspectWorktrees()
	}
	if s == "p" {
		return m.inspectPush()
	}
	if s == "ctrl+g" {
		m.openSuggestion()
		return nil
	}
	if s == "ctrl+s" || s == "alt+enter" {
		return m.review()
	}
	if s == "c" {
		if m.focusPane == "commit" && m.activeRightTab() == "commit" {
			return m.review()
		}
		m.rightTab = "commit"
		m.focusPane = "commit"
		m.commitField = "summary"
		m.commitEditing = true
		return nil
	}
	if m.focusPane == "commit" {
		switch s {
		case "enter":
			if m.commitField == "actions" {
				return m.review()
			}
			m.commitEditing = true
		case "up", "k":
			m.commitField = "summary"
		case "down", "j":
			m.commitField = "description"
		}
		return nil
	}
	if s == "s" && !m.loading {
		return m.stage()
	}
	if s == "A" {
		return m.stageAll(false)
	}
	if s == "U" {
		return m.stageAll(true)
	}
	if s == "x" || s == "d" {
		return m.inspectDiscard()
	}
	if s == "pgup" && m.focusPane == "tree" {
		m.moveTree(-m.bodyHeight())
		return m.loadSelected()
	}
	if s == "pgdown" && m.focusPane == "tree" {
		m.moveTree(m.bodyHeight())
		return m.loadSelected()
	}
	if s == "pgup" {
		m.viewer.scroll(-m.bodyHeight(), m.bodyHeight())
		return nil
	}
	if s == "pgdown" {
		m.viewer.scroll(m.bodyHeight(), m.bodyHeight())
		return nil
	}
	if m.focusPane == "viewer" {
		switch s {
		case "up", "k":
			m.viewer.scroll(-1, m.bodyHeight())
		case "down", "j":
			m.viewer.scroll(1, m.bodyHeight())
		case "left", "h":
			m.viewer.column = max(0, m.viewer.column-4)
		case "right", "l":
			m.viewer.column += 4
		case "home", "g":
			m.viewer.offset = 0
		case "end", "G":
			m.viewer.offset = max(0, len(m.viewer.lines)-m.bodyHeight())
		}
		return nil
	}
	if m.loading {
		return nil
	}
	switch s {
	case "up", "k":
		m.moveTree(-1)
		return m.loadSelected()
	case "down", "j":
		m.moveTree(1)
		return m.loadSelected()
	case "home", "g":
		m.selected = 0
		if len(m.rows) > 0 && m.rows[0].node == nil {
			m.moveTree(1)
		}
		m.keepTreeVisible()
		return m.loadSelected()
	case "end", "G":
		m.selected = max(0, len(m.rows)-1)
		if len(m.rows) > 0 && m.rows[m.selected].node == nil {
			m.moveTree(-1)
		}
		m.keepTreeVisible()
		return m.loadSelected()
	case "enter", " ", "right", "l":
		if node := m.selectedNode(); node != nil {
			if node.IsDir {
				k := m.expandedKey()
				m.expanded[k] = !m.expanded[k]
				m.rebuildTree()
			} else {
				m.focusPane = "viewer"
			}
		}
	case "left", "h":
		if node := m.selectedNode(); node != nil {
			if node.IsDir && m.expanded[m.expandedKey()] {
				m.expanded[m.expandedKey()] = false
				m.rebuildTree()
			} else {
				for i := m.selected - 1; i >= 0; i-- {
					if m.rows[i].node != nil && m.rows[i].group == m.rows[m.selected].group && m.rows[i].depth < m.rows[m.selected].depth {
						m.selected = i
						m.keepTreeVisible()
						return m.loadSelected()
					}
				}
			}
		}
	}
	return nil
}

func (m *Model) openSuggestion() {
	if !m.canAct() {
		return
	}
	if m.changeSummary().Staged == 0 {
		m.notice = "Nenhum arquivo no stage para sugerir mensagem."
		return
	}
	m.rightTab = "commit"
	m.modal = "suggest"
	m.modalScroll = 0
	m.commitPhase = "type"
	m.commitEditing = false
}
func (m *Model) cycleFocus() {
	m.commitEditing = false
	switch m.focusPane {
	case "tree":
		m.focusPane = "viewer"
	case "viewer":
		m.focusPane = "commit"
	default:
		m.focusPane = "tree"
	}
}
func (m *Model) cycleCommitField() {
	switch m.commitField {
	case "summary":
		m.commitField = "description"
		m.commitEditing = true
	case "description":
		m.commitField = "actions"
		m.commitEditing = false
	default:
		m.commitField = "summary"
		m.commitEditing = true
	}
}
func (m *Model) setViewerTab(tab string) {
	if m.viewerTab == tab {
		return
	}
	m.viewerTab = tab
	m.viewer.setMode(tab)
}
