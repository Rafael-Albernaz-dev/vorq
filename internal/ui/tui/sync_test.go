package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSyncButtonMergesRemoteBase(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	for _, args := range [][]string{
		{"add", "-A"}, {"commit", "-m", "chore: base"}, {"push", "-u", "origin", "main"},
		{"commit", "--allow-empty", "-m", "feat: on main"}, {"push", "origin", "main"},
		{"switch", "-c", "feat/x", "HEAD~1"}, {"commit", "--allow-empty", "-m", "feat: local"},
	} {
		if _, err := r.Exec(ctx, dir, args...); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, size := range [][2]int{{160, 42}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		drive(t, m, m.refresh())
		x, y := mouseLabelPosition(t, m.renderView(), syncButton)
		drive(t, m, mousePress(m, x+1, y))
		if m.modal != modalSync || m.syncTarget == nil || m.syncTarget.Base != "main" || m.syncTarget.Behind != 1 {
			t.Fatalf("size %v: modal %q target %+v notice %q", size, m.modal, m.syncTarget, m.notice)
		}
		assertFrame(t, m)
		drive(t, m, press(m, "esc"))
	}
	x, y := mouseLabelPosition(t, m.renderView(), syncButton)
	drive(t, m, mousePress(m, x+1, y))
	drive(t, m, press(m, "enter"))
	if m.modal != modalResult || m.resultKind != "sync" || !strings.Contains(m.resultBody, "1 commit(s) de origin/main") {
		t.Fatalf("modal %q body %q", m.modal, m.resultBody)
	}
	subject, _ := r.Exec(ctx, dir, "log", "-1", "--format=%s")
	if !strings.Contains(subject, "Merge remote-tracking branch 'origin/main' into feat/x") {
		t.Fatalf("HEAD %q", subject)
	}
}

func TestSyncButtonOnBaseOpensPull(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "chore: base"}, {"push", "-u", "origin", "main"}} {
		if _, err := r.Exec(ctx, dir, args...); err != nil {
			t.Fatal(args, err)
		}
	}
	drive(t, m, m.refresh())
	x, y := mouseLabelPosition(t, m.renderView(), syncButton)
	drive(t, m, mousePress(m, x+1, y))
	if m.modal != modalPull {
		t.Fatalf("modal %q notice %q", m.modal, m.notice)
	}
}
