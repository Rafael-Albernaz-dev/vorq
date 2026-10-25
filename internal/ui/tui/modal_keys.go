package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) handleModalKey(key tea.KeyMsg) tea.Cmd {
	s := key.String()
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
	case "pull":
		if s == "enter" {
			return m.pull()
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
