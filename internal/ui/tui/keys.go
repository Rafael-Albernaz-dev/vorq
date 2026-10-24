package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKey(key tea.KeyMsg) tea.Cmd {
	s := key.String()
	if s == "ctrl+c" {
		return tea.Quit
	}
	if m.modal != "" {
		if m.modal != "worktrees" && (s == "pgdown" || s == "down" || s == "j") {
			delta := 1
			if s == "pgdown" {
				delta = m.modalBodyHeight()
			}
			m.modalScroll += delta
			return nil
		}
		if m.modal != "worktrees" && (s == "pgup" || s == "up" || s == "k") {
			delta := 1
			if s == "pgup" {
				delta = m.modalBodyHeight()
			}
			m.modalScroll = max(0, m.modalScroll-delta)
			return nil
		}
		if s == "esc" {
			if m.mutating {
				m.notice = "Operação em andamento. Ctrl+C encerra e cancela o processo."
				return nil
			}
			m.dialogID++
			if m.dialogCancel != nil {
				m.dialogCancel()
			}
			m.discardPlan = nil
			m.busy = ""
			if m.modal == "commit" || m.modal == "suggest" {
				m.notice = "Revisão cancelada. Os arquivos permanecem em stage."
			}
			m.modal = ""
			return nil
		}
		if s == "q" {
			return tea.Quit
		}
		if m.busy != "" {
			return nil
		}
		switch m.modal {
		case "appearance":
			if s == "i" {
				m.cycleIcons()
			}
			for i := range studioPalettes {
				if s == string(rune('1'+i)) {
					return m.selectTheme(i)
				}
			}
		case "discard":
			if s == "enter" {
				return m.discard(false)
			}
			if s == "u" {
				return m.discard(true)
			}
		case "unstage":
			if s == "enter" {
				if n := m.selectedNode(); n != nil {
					m.partialApproved = n.Path
				}
				m.modal = ""
				return m.stage()
			}
		case "help", "error", "result":
			if s == "?" || s == "enter" {
				m.modal = ""
			}
		case "worktrees":
			switch s {
			case "up", "k":
				m.worktreeIndex = max(0, m.worktreeIndex-1)
			case "down", "j":
				m.worktreeIndex = min(max(0, len(m.worktrees)-1), m.worktreeIndex+1)
			case "enter":
				if len(m.worktrees) > 0 {
					w := m.worktrees[m.worktreeIndex]
					if w.IsBare || w.IsPrunable {
						m.notice = "Worktree indisponível para navegação."
						return nil
					}
					return m.switchWorkspace(w.Path)
				}
			}
		case "push":
			if s == "enter" {
				return m.push()
			}
		case "suggest":
			for i, value := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
				if s == value {
					return m.propose([]string{"feat", "fix", "chore", "refactor", "docs", "test", "style", "perf"}[i])
				}
			}
		case "commit":
			if s == "enter" && m.commitPhase == "review" {
				return m.commit()
			}
			if s == "e" {
				m.modal = ""
				m.focusPane = "commit"
				m.commitField = "summary"
				m.commitEditing = true
			}
		}
		return nil
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
		if m.focusPane == "commit" {
			return m.review()
		}
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

func (m *Model) editKey(key tea.KeyMsg) {
	value, cursor := &m.commitSummary, &m.summaryCursor
	if m.commitField == "description" {
		value, cursor = &m.commitDescription, &m.descriptionCursor
	}
	runes := []rune(*value)
	*cursor = min(max(0, *cursor), len(runes))
	pos := *cursor
	changed := false
	start := pos
	for start > 0 && runes[start-1] != '\n' {
		start--
	}
	end := pos
	for end < len(runes) && runes[end] != '\n' {
		end++
	}
	switch key.String() {
	case "enter":
		if m.commitField == "summary" {
			m.commitField = "description"
			return
		}
		runes = append(append(append([]rune(nil), runes[:pos]...), '\n'), runes[pos:]...)
		*cursor++
		changed = true
	case "left":
		*cursor = max(0, pos-1)
	case "right":
		*cursor = min(len(runes), pos+1)
	case "home", "ctrl+a":
		*cursor = start
	case "end", "ctrl+e":
		*cursor = end
	case "up":
		if start > 0 {
			previous := start - 1
			for previous > 0 && runes[previous-1] != '\n' {
				previous--
			}
			*cursor = previous + min(pos-start, start-1-previous)
		}
	case "down":
		if end < len(runes) {
			nextEnd := end + 1
			for nextEnd < len(runes) && runes[nextEnd] != '\n' {
				nextEnd++
			}
			*cursor = end + 1 + min(pos-start, nextEnd-end-1)
		}
	case "ctrl+u":
		runes = nil
		*cursor = 0
		changed = true
	case "backspace", "ctrl+h":
		if pos > 0 {
			runes = append(runes[:pos-1], runes[pos:]...)
			*cursor--
			changed = true
		}
	case "delete":
		if pos < len(runes) {
			runes = append(runes[:pos], runes[pos+1:]...)
			changed = true
		}
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			inserted := key.Runes
			if key.Type == tea.KeySpace {
				inserted = []rune{' '}
			}
			inserted = []rune(strings.Map(func(r rune) rune {
				if r == '\n' && m.commitField == "description" {
					return r
				}
				if r == '\n' {
					return ' '
				}
				if unicode.IsControl(r) {
					return -1
				}
				return r
			}, string(inserted)))
			limit := 65536
			if m.commitField == "summary" {
				limit = 4096
			}
			if len(runes)+len(inserted) > limit {
				return
			}
			tail := append([]rune(nil), runes[pos:]...)
			runes = append(runes[:pos], inserted...)
			runes = append(runes, tail...)
			*cursor += len(inserted)
			changed = true
		}
	}
	if changed {
		*value = string(runes)
		m.draftRevision++
	}
}
