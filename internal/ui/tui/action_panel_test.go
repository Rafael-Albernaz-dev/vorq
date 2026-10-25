package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func rightOrigin(m *Model) (int, int) {
	if !m.isWide {
		return 0, m.width
	}
	left, center, right := m.paneWidths()
	return left + center + 2, right
}
func clickPanelTab(t *testing.T, m *Model, index int) {
	t.Helper()
	if !m.isWide {
		mousePress(m, m.width*5/6, 3)
	}
	origin, width := rightOrigin(m)
	widths := tabWidths(rightTabs, width)
	x := origin
	for i := 0; i < index; i++ {
		x += widths[i]
	}
	drive(t, m, mousePress(m, x+widths[index]/2, m.bodyTop()))
}
func clickPanelAction(t *testing.T, m *Model, label string) tea.Cmd {
	t.Helper()
	origin, width := rightOrigin(m)
	x, y := mouseLabelPosition(t, m.commitPane(width), "["+label+"]")
	return mousePress(m, origin+x+1, m.bodyTop()+y)
}

func TestActionTabsHaveNoIOAndPreserveDraftAtAllSizes(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	stub := &workflowStub{StudioService: m.core}
	m.core = stub
	m.commitSummary = "feat: preserve"
	m.commitDescription = "body\nsecond"
	for _, size := range [][2]int{{160, 42}, {140, 36}, {80, 24}, {50, 14}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for index, id := range rightTabIDs {
			clickPanelTab(t, m, index)
			if m.activeRightTab() != id || m.modal != "" || m.screen != "" {
				t.Fatalf("size=%v tab=%s", size, id)
			}
			_, width := rightOrigin(m)
			view := m.commitPane(width)
			if len(strings.Split(view, "\n")) != m.panelHeight() {
				t.Fatal("panel height changed")
			}
			for _, line := range strings.Split(view, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatal("panel overflow")
				}
			}
		}
		if m.commitSummary != "feat: preserve" || m.commitDescription != "body\nsecond" {
			t.Fatal("draft lost")
		}
	}
	if stub.lists != 0 || stub.views != 0 || stub.saves != 0 || core.stageCalls != 0 || core.pushCalls != 0 || core.commitCalls != 0 {
		t.Fatal("tab selection performed IO")
	}
}
func TestActionPanelMouseOpensBranchHistoryWorktreesAndPRs(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	stub := &workflowStub{StudioService: m.core}
	m.core = stub
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	clickPanelTab(t, m, 1)
	drive(t, m, clickPanelAction(t, m, "Nova branch"))
	if m.modal != "workflow" || m.formKind != "branch" {
		t.Fatal("branch form not opened")
	}
	press(m, "esc")
	drive(t, m, clickPanelAction(t, m, "Histórico de commits"))
	if m.screen != "history" {
		t.Fatal("history not opened")
	}
	press(m, "esc")
	clickPanelTab(t, m, 3)
	drive(t, m, clickPanelAction(t, m, "Abrir worktrees"))
	if m.modal != "worktrees" {
		t.Fatal("worktree chooser not opened")
	}
	press(m, "esc")
	clickPanelTab(t, m, 2)
	if stub.lists != 0 {
		t.Fatal("PR tab queried GitHub")
	}
	drive(t, m, clickPanelAction(t, m, "Listar PRs"))
	if m.screen != "prs" || stub.lists != 1 {
		t.Fatal("PR list not opened")
	}
	press(m, "esc")
	clickPanelTab(t, m, 2)
	drive(t, m, clickPanelAction(t, m, "Criar PR"))
	if m.modal != "workflow" || m.formKind != "pr" || stub.saves != 0 {
		t.Fatal("PR form must not publish")
	}
	if core.pushCalls != 0 || core.commitCalls != 0 {
		t.Fatal("panel action published")
	}
}

type panelPullStub struct {
	app.StudioService
	previews, pulls int
}

func (s *panelPullStub) GetPullTarget(context.Context, string) (*domain.PullTarget, error) {
	s.previews++
	return &domain.PullTarget{Branch: "main", Remote: "origin", RemoteBranch: "main", Behind: 1}, nil
}
func (s *panelPullStub) PullChanges(context.Context, string, domain.PullTarget) (*domain.PullResult, error) {
	s.pulls++
	return &domain.PullResult{Output: "Fast-forward"}, nil
}
func TestContextArrowClicksOnlyPreviewAndDoNotAcceptBranchOrPathText(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	stub := &panelPullStub{StudioService: m.core}
	m.core = stub
	for _, size := range [][2]int{{160, 42}, {140, 36}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.rightTab = "commit"
		m.focusPane = "tree"
		x, y := mouseLabelPosition(t, m.View(), "↑0")
		drive(t, m, mousePress(m, x, y))
		if m.modal != "push" || core.pushCalls != 0 {
			t.Fatal("up arrow did not open push preview")
		}
		press(m, "esc")
		x, y = mouseLabelPosition(t, m.View(), "↓0")
		drive(t, m, mousePress(m, x, y))
		if m.modal != "pull" || stub.pulls != 0 {
			t.Fatal("down arrow must only preview")
		}
		press(m, "esc")
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	m.studio.Workspace.Branch = "fix/↑0"
	x, y := mouseLabelPosition(t, m.View(), "↑0")
	if cmd := mousePress(m, x, y); cmd != nil || m.modal != "" {
		t.Fatal("branch name became push control")
	}
	m.studio.Workspace.Branch = "main"
	m.dir = "/tmp/↑0"
	x, y = mouseLabelPosition(t, m.View(), "↑0")
	if cmd := mousePress(m, x, y); cmd != nil || m.modal != "" {
		t.Fatal("workspace path became push control")
	}
}
func TestPullButtonRequiresConfirmationAndBlocksDuplicate(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	stub := &panelPullStub{StudioService: m.core}
	m.core = stub
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	clickPanelTab(t, m, 1)
	drive(t, m, clickPanelAction(t, m, "Atualizar branch ↓"))
	if stub.previews != 1 || stub.pulls != 0 {
		t.Fatal("preview performed pull")
	}
	cmd := press(m, "enter")
	if cmd == nil || !m.mutating {
		t.Fatal("confirmation missing")
	}
	if duplicate := press(m, "enter"); duplicate != nil {
		t.Fatal("duplicate pull scheduled")
	}
	drive(t, m, cmd)
	if stub.pulls != 1 || m.modal != "result" {
		t.Fatal("confirmed pull not completed")
	}
}
func TestActionTabsKeyboardAndTextPriority(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	clickPanelTab(t, m, 1)
	press(m, "right")
	if m.activeRightTab() != "prs" {
		t.Fatal("keyboard tab navigation missing")
	}
	press(m, "c")
	if m.activeRightTab() != "commit" || !m.commitEditing {
		t.Fatal("c did not return to visible commit draft")
	}
	press(m, "right")
	if m.activeRightTab() != "commit" {
		t.Fatal("editing cursor changed tabs")
	}
}
