package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func clickSwitchModalLabel(t *testing.T, m *Model, label string) tea.Cmd {
	t.Helper()
	view := m.modalView()
	rows := strings.Split(view, "\n")
	width := 0
	for _, line := range rows {
		width = max(width, ansi.StringWidth(line))
	}
	x, y := mouseLabelPosition(t, view, label)
	return mousePress(m, max(0, (m.width-width)/2)+x+1, max(0, (m.height-len(rows))/2)+y)
}
func TestBranchSwitchMouseReviewCancelAndConfirm(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "test: clean fixture")
	if _, e := r.Exec(ctx, dir, "branch", "feature"); e != nil {
		t.Fatal(e)
	}
	drive(t, m, m.refresh())
	m.commitSummary = "feat: draft stays"
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	clickPanelTab(t, m, 1)
	for _, confirm := range []bool{false, true} {
		drive(t, m, clickPanelAction(t, m, "Trocar branch"))
		if m.modal != "branches" {
			t.Fatal("branch selector absent")
		}
		drive(t, m, clickSwitchModalLabel(t, m, "feature"))
		branch, _ := r.CurrentBranch(ctx, dir)
		if branch != "main" {
			t.Fatal("row click switched branch")
		}
		drive(t, m, clickSwitchModalLabel(t, m, "[Revisar]"))
		if m.modal != "switch-branch" || m.branchSwitchPlan == nil {
			t.Fatal("review absent")
		}
		branch, _ = r.CurrentBranch(ctx, dir)
		if branch != "main" {
			t.Fatal("review switched branch")
		}
		if !confirm {
			press(m, "esc")
			continue
		}
		cmd := clickSwitchModalLabel(t, m, "[Confirmar]")
		if duplicate := press(m, "enter"); duplicate != nil {
			t.Fatal("duplicate switch scheduled")
		}
		drive(t, m, cmd)
	}
	branch, _ := r.CurrentBranch(ctx, dir)
	if branch != "feature" || m.commitSummary != "feat: draft stays" {
		t.Fatal("switch/draft preservation failed")
	}
}
func TestBranchSwitchDirtyBlockedAndLateReplyIgnored(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	_, _ = r.Exec(context.Background(), dir, "branch", "feature")
	drive(t, m, m.inspectBranches())
	id := m.dialogID
	var selected int
	for i, b := range m.localBranches {
		if b.Name == "feature" {
			selected = i
		}
	}
	m.branchIndex = selected
	drive(t, m, m.reviewBranchSwitch())
	if m.modal != "error" || !strings.Contains(m.lastError, "workspace deve estar limpo") {
		t.Fatal("dirty branch switch not blocked")
	}
	press(m, "esc")
	m.Update(branchesMsg{id: id})
	if m.modal != "" {
		t.Fatal("old response reopened selector")
	}
	branch, _ := r.CurrentBranch(context.Background(), dir)
	if branch != "main" {
		t.Fatal("dirty branch changed")
	}
}
func TestBranchSwitchLayoutsAndButtonReachability(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "test: clean fixture")
	_, _ = r.Exec(ctx, dir, "branch", "feature")
	drive(t, m, m.refresh())
	for _, size := range [][2]int{{50, 14}, {80, 24}, {160, 42}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		clickPanelTab(t, m, 1)
		drive(t, m, clickPanelAction(t, m, "Trocar branch"))
		for _, line := range strings.Split(m.modalView(), "\n") {
			if ansi.StringWidth(line) > m.width {
				t.Fatal("selector overflow")
			}
		}
		if len(strings.Split(m.modalView(), "\n")) > m.height {
			t.Fatal("selector too tall")
		}
		if !strings.Contains(ansi.Strip(m.modalView()), "[Revisar]") {
			t.Fatal("review button hidden")
		}
		press(m, "esc")
	}
}

func TestBranchSwitchLoadingCancelByMouse(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	cmd := m.inspectBranches()
	if cmd == nil {
		t.Fatal("branch read not scheduled")
	}
	drive(t, m, clickSwitchModalLabel(t, m, "[Voltar]"))
	if m.modal != "" || m.busy != "" {
		t.Fatal("loading modal cannot be canceled by mouse")
	}
	m.Update(cmd())
	if m.modal != "" {
		t.Fatal("canceled list response restored modal")
	}
}
