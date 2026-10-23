package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleKey(key tea.KeyMsg) tea.Cmd {
	s := key.String()
	if s == "ctrl+c" {
		return tea.Quit
	}
	if s == "q" && !(m.modal == "commit" && m.commitPhase == "edit") {
		return tea.Quit
	}
	if m.modal != "" {
		if s == "esc" {
			if m.mutating {
				m.notice = "Operação em andamento. Ctrl+C encerra e cancela o processo."
				return nil
			}
			if m.modal == "commit" && m.commitPhase == "edit" {
				m.commitPhase = "review"
				return nil
			}
			if m.modal == "commit" {
				m.notice = "Commit cancelado. Os arquivos permanecem em stage."
			}
			m.modal = ""
			return nil
		}
		if m.busy != "" {
			return nil
		}
		switch m.modal {
		case "help":
			if s == "?" || s == "q" {
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
					m.dir = w.Path
					m.modal = ""
					m.studio = nil
					m.rows = nil
					m.expanded = map[string]bool{}
					m.selected = 0
					m.treeOffset = 0
					m.viewer = viewerState{}
					m.viewer.rebuild()
					return m.refresh()
				}
			}
		case "push":
			if s == "enter" {
				return m.push()
			}
		case "commit":
			if m.commitPhase == "type" {
				for i, value := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
					if s == value {
						return m.propose([]string{"feat", "fix", "chore", "refactor", "docs", "test", "style", "perf"}[i])
					}
				}
			} else if m.commitPhase == "edit" {
				m.editKey(key)
			} else {
				if s == "e" {
					m.commitPhase = "edit"
					m.edit = []rune(m.commitMessage)
					m.cursor = len(m.edit)
				}
				if s == "enter" {
					return m.commit()
				}
			}
		}
		return nil
	}
	if s == "q" {
		return tea.Quit
	}
	if s == "?" {
		m.modal = "help"
		return nil
	}
	if s == "i" {
		m.nerdFonts = !m.nerdFonts
		return nil
	}
	if s == "shift+tab" {
		if m.focus == "tree" {
			m.focus = "viewer"
		} else {
			m.focus = "tree"
		}
		return nil
	}
	if s == "tab" {
		if m.viewer.mode == "diff" {
			m.viewer.mode = "arquivo"
		} else {
			m.viewer.mode = "diff"
		}
		m.viewer.rebuild()
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
	if m.loading {
		return nil
	}
	if s == "s" {
		return m.stage()
	}
	if s == "c" {
		m.modal = "commit"
		m.commitPhase = "type"
		return nil
	}
	if s == "p" {
		return m.inspectPush()
	}
	if s == "pgup" {
		m.viewer.scroll(-m.bodyHeight(), m.bodyHeight())
		return nil
	}
	if s == "pgdown" {
		m.viewer.scroll(m.bodyHeight(), m.bodyHeight())
		return nil
	}
	if m.focus == "viewer" {
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
	switch s {
	case "up", "k":
		m.moveTree(-1)
		return m.loadSelected()
	case "down", "j":
		m.moveTree(1)
		return m.loadSelected()
	case "home", "g":
		m.selected = 0
		m.keepTreeVisible()
		return m.loadSelected()
	case "end", "G":
		m.selected = max(0, len(m.rows)-1)
		m.keepTreeVisible()
		return m.loadSelected()
	case "enter", " ", "right", "l":
		if node := m.selectedNode(); node != nil {
			if node.IsDir {
				m.expanded[node.Path] = !m.expanded[node.Path]
				m.rebuildTree()
			} else {
				m.focus = "viewer"
			}
		}
	case "left", "h":
		if node := m.selectedNode(); node != nil {
			if node.IsDir && m.expanded[node.Path] {
				m.expanded[node.Path] = false
				m.rebuildTree()
			} else {
				for i := m.selected - 1; i >= 0; i-- {
					if m.rows[i].depth < m.rows[m.selected].depth {
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

func (m *Model) editKey(key tea.KeyMsg) {
	switch key.String() {
	case "enter":
		if strings.TrimSpace(string(m.edit)) != "" {
			m.commitMessage = string(m.edit)
			m.commitPhase = "review"
		}
	case "left":
		m.cursor = max(0, m.cursor-1)
	case "right":
		m.cursor = min(len(m.edit), m.cursor+1)
	case "home", "ctrl+a":
		m.cursor = 0
	case "end", "ctrl+e":
		m.cursor = len(m.edit)
	case "ctrl+u":
		m.edit = nil
		m.cursor = 0
	case "backspace", "ctrl+h":
		if m.cursor > 0 {
			m.edit = append(m.edit[:m.cursor-1], m.edit[m.cursor:]...)
			m.cursor--
		}
	case "delete":
		if m.cursor < len(m.edit) {
			m.edit = append(m.edit[:m.cursor], m.edit[m.cursor+1:]...)
		}
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			runes := key.Runes
			if key.Type == tea.KeySpace {
				runes = []rune{' '}
			}
			if len(m.edit)+len(runes) > 4096 {
				return
			}
			tail := append([]rune(nil), m.edit[m.cursor:]...)
			m.edit = append(m.edit[:m.cursor], runes...)
			m.edit = append(m.edit, tail...)
			m.cursor += len(runes)
		}
	}
}
