package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestLayoutMinimumDialogsKeepActionsAndEveryLineReachable(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 14})
	for _, modal := range []string{"help", "error", "appearance", "suggest", "commit", "push", "worktrees", "unstage", "result"} {
		m.modal = modal
		m.commitPhase = "review"
		m.reviewMessage = "feat: real review"
		m.lastError = "native error"
		m.resultBody = "real operation result"
		rows := strings.Split(m.modalView(), "\n")
		if len(rows) > m.height {
			t.Errorf("%s dialog height %d exceeds %d", modal, len(rows), m.height)
		}
		for _, line := range rows {
			if ansi.StringWidth(line) > m.width {
				t.Errorf("%s dialog too wide", modal)
			}
		}
		if !strings.Contains(ansi.Strip(m.View()), "[Voltar]") {
			t.Errorf("%s lost cancel/close action", modal)
		}
	}
	m.modal = "help"
	m.modalScroll = 0
	press(m, "down")
	if m.modalScroll != 1 {
		t.Fatal("line navigation must not skip review lines at minimum height")
	}
	press(m, "pgdown")
	if m.modalScroll != 1+m.modalBodyHeight() {
		t.Fatal("page navigation must use actual modal viewport")
	}
}
