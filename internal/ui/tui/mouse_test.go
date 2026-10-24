package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func mousePress(m *Model, x, y int) tea.Cmd {
	_, cmd := m.mouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	return cmd
}

func mouseLabelPosition(t *testing.T, rendered, label string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(rendered, "\n") {
		plain := ansi.Strip(line)
		if index := strings.Index(plain, label); index >= 0 {
			return ansi.StringWidth(plain[:index]), y
		}
	}
	t.Fatalf("visible mouse control %q missing", label)
	return 0, 0
}

func TestMouse_TabsCompactNavigationAndReadOnlySelection(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	for _, size := range [][2]int{{140, 36}, {160, 42}, {80, 24}, {50, 14}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.focusPane, m.activeTab = "tree", "alteracoes"
		m.rebuildTree()
		x, y := mouseLabelPosition(t, m.View(), "Repositório")
		drive(t, m, mousePress(m, x+2, y))
		if m.activeTab != "repositorio" || core.stageCalls != 0 {
			t.Fatalf("size %v: tab click failed or staged a path", size)
		}
		if !m.isWide {
			mousePress(m, size[0]/2, 3)
			if m.focusPane != "viewer" {
				t.Fatalf("size %v: explicit compact viewer navigation failed", size)
			}
		}
		x, y = mouseLabelPosition(t, m.View(), "Stage")
		mousePress(m, x+1, y)
		if m.viewerTab != "stage" || core.stageCalls != 0 {
			t.Fatalf("size %v: viewer tab changed repository state", size)
		}
	}
}

func TestMouse_StagePressIsAsyncAndReleaseDoesNotRepeat(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	selectPath(t, m, "file.go")
	y := m.bodyTop() + 4 + m.selected - m.treeOffset
	cmd := mousePress(m, 4, y)
	if cmd == nil || core.stageCalls != 0 {
		t.Fatal("stage must be returned as a command, not run in mouse update")
	}
	m.mouse(tea.MouseMsg{X: 4, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m.mouse(tea.MouseMsg{X: 4, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	if duplicate := mousePress(m, 4, y); duplicate != nil {
		t.Fatal("busy mouse press scheduled duplicate staging")
	}
	drive(t, m, cmd)
	if core.stageCalls != 1 || !m.selectedNode().Status.IsStaged() {
		t.Fatal("single checkbox press did not stage exactly the selected path")
	}
}

func TestMouse_WheelBelongsToHoveredPaneAndModal(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	writeStudio(t, dir, "file.go", strings.Repeat("// real text\n", 150))
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	left, _, _ := m.paneWidths()
	m.mouse(tea.MouseMsg{X: 10, Y: 10, Button: tea.MouseButtonWheelDown})
	if m.viewer.offset != 0 {
		t.Fatal("wheel over tree leaked into viewer")
	}
	m.mouse(tea.MouseMsg{X: left + 8, Y: 10, Button: tea.MouseButtonWheelDown})
	if m.viewer.offset != 3 {
		t.Fatal("viewer wheel did not scroll hovered viewer")
	}
	m.modal = "help"
	before := m.viewer.offset
	m.mouse(tea.MouseMsg{X: left + 8, Y: 10, Button: tea.MouseButtonWheelDown})
	if m.modalScroll != 3 || m.viewer.offset != before {
		t.Fatal("modal failed to capture wheel")
	}
	beforeTab := m.activeTab
	mousePress(m, 15, 4)
	if m.activeTab != beforeTab || m.modal != "help" {
		t.Fatal("background click escaped modal ownership")
	}
}

func TestMouse_CommitFieldsUseCellsAndPreserveTyping(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	left, center, _ := m.paneWidths()
	origin := left + center + 2
	m.commitSummary, m.summaryCursor = "fix: ação diária", 0
	mousePress(m, origin+2+10, m.bodyTop()+6)
	if m.focusPane != "commit" || !m.commitEditing || m.commitField != "summary" || m.summaryCursor != 10 {
		t.Fatalf("accented summary mouse cursor/focus: %s %v %s %d", m.focusPane, m.commitEditing, m.commitField, m.summaryCursor)
	}
	press(m, "qs")
	if m.commitSummary != "fix: ação qsdiária" || core.stageCalls != 0 {
		t.Fatalf("letters triggered actions or wrong cursor: %q", m.commitSummary)
	}
	m.commitDescription = strings.Repeat("descrição real\n", 20)
	m.descriptionOffset = 4
	mousePress(m, origin+2+3, m.bodyTop()+10)
	if m.commitField != "description" || m.descriptionCursor != len([]rune(strings.Repeat("descrição real\n", 4)))+3 {
		t.Fatalf("description offset/cursor not respected: %d", m.descriptionCursor)
	}
	before := m.commitDescription
	press(m, "enter")
	if strings.Count(m.commitDescription, "\n") != strings.Count(before, "\n")+1 || core.commitCalls != 0 {
		t.Fatal("description Enter confirmed commit instead of inserting a line")
	}
}

func TestFieldCursorAt_WrappingAccentsAndWideCells(t *testing.T) {
	for _, tc := range []struct {
		value                            string
		width, row, column, offset, want int
	}{
		{"ação", 20, 0, 3, 0, 3},
		{"abc\ndef", 20, 1, 2, 0, 6},
		{"abc\ndef", 20, 0, 2, 1, 6},
		{"ab界c", 20, 0, 3, 0, 2},
		{"ab界c", 20, 0, 4, 0, 3},
		{"one two three", 7, 1, 2, 0, 10},
	} {
		if got := fieldCursorAt(tc.value, tc.width, tc.row, tc.column, tc.offset); got != tc.want {
			t.Fatalf("%q row=%d col=%d offset=%d: got %d want %d", tc.value, tc.row, tc.column, tc.offset, got, tc.want)
		}
	}
}

func TestMouse_ReviewCancelConfirmAndExplicitPush(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	selectPath(t, m, "file.go")
	drive(t, m, mousePress(m, 4, m.bodyTop()+4+m.selected-m.treeOffset))
	m.commitSummary = "fix: mouse confirmation"
	x, y := mouseLabelPosition(t, m.View(), "[c] Revisar commit local")
	drive(t, m, mousePress(m, x+2, y))
	if m.modal != "commit" || m.commitPhase != "review" || core.commitCalls != 0 {
		t.Fatal("review click committed without explicit confirmation")
	}
	x, y = mouseLabelPosition(t, m.View(), "[Voltar]")
	mousePress(m, x+2, y)
	if m.modal != "" || m.commitSummary == "" || !m.selectedNode().Status.IsStaged() {
		t.Fatal("cancel click lost draft or stage")
	}
	x, y = mouseLabelPosition(t, m.View(), "[c] Revisar commit local")
	drive(t, m, mousePress(m, x+2, y))
	x, y = mouseLabelPosition(t, m.View(), "[Confirmar]")
	cmd := mousePress(m, x+2, y)
	if cmd == nil || core.commitCalls != 0 {
		t.Fatal("confirm must schedule native commit asynchronously")
	}
	m.mouse(tea.MouseMsg{X: x + 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	drive(t, m, cmd)
	if core.commitCalls != 1 || core.pushCalls != 0 || m.commitSummary != "" {
		t.Fatal("confirmed commit duplicated, retained draft or pushed implicitly")
	}
	if m.modal != "result" || m.resultKind != "commit" || !strings.Contains(m.resultBody, "Commit local") {
		t.Fatal("real commit result was not presented")
	}
	x, y = mouseLabelPosition(t, m.View(), "[Voltar]")
	mousePress(m, x+1, y)
	drive(t, m, press(m, "p"))
	if m.pushTarget == nil || core.pushCalls != 0 {
		t.Fatal("push preview missing or published implicitly")
	}
	x, y = mouseLabelPosition(t, m.View(), "[Confirmar]")
	cmd = mousePress(m, x+2, y)
	if cmd == nil || core.pushCalls != 0 {
		t.Fatal("push click did not schedule asynchronous publication")
	}
	drive(t, m, cmd)
	if core.pushCalls != 1 {
		t.Fatal("confirmed push was not executed once")
	}
}

func TestMouse_AppearanceAndDescriptionWheel(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	press(m, "t")
	x, y := mouseLabelPosition(t, m.View(), "[4]")
	mousePress(m, x+1, y)
	if m.themeIndex != 3 {
		t.Fatal("appearance palette click not handled")
	}
	x, y = mouseLabelPosition(t, m.View(), "[i]")
	mousePress(m, x+1, y)
	if m.currentIconMode() != IconModeText {
		t.Fatal("appearance icon click not handled")
	}
	x, y = mouseLabelPosition(t, m.View(), "[Voltar]")
	mousePress(m, x+1, y)
	m.focusPane, m.commitField, m.commitEditing = "commit", "description", true
	m.commitDescription = strings.Repeat("scroll body\n", 30)
	left, center, _ := m.paneWidths()
	m.mouse(tea.MouseMsg{X: left + center + 6, Y: m.bodyTop() + 10, Button: tea.MouseButtonWheelDown})
	if m.descriptionOffset != 3 || m.commitEditing {
		t.Fatal("description wheel failed to expose its scroll viewport")
	}
}

func TestMouse_WorktreeRowSelectsBeforeExplicitOpen(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	other := filepath.Join(t.TempDir(), "linked")
	if err := r.WorktreeAdd(context.Background(), dir, "feature", other, "main", false); err != nil {
		t.Fatal(err)
	}
	drive(t, m, press(m, "w"))
	x, y := mouseLabelPosition(t, m.View(), "feature")
	mousePress(m, x+2, y)
	if m.dir != dir || m.worktrees[m.worktreeIndex].Path != other {
		t.Fatal("worktree row failed to select or switched without Open")
	}
	x, y = mouseLabelPosition(t, m.View(), "[Abrir]")
	cmd := mousePress(m, x+2, y)
	if cmd == nil {
		t.Fatal("Open did not schedule workspace reload")
	}
	drive(t, m, cmd)
	if m.dir != other || m.studio.Workspace.Branch != "feature" {
		t.Fatal("explicit worktree Open failed")
	}
}

func TestMouse_SamePathInBothGroupsSelectsCorrespondingDiff(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	drive(t, m, press(m, "s"))
	writeStudio(t, dir, "file.go", "package main\n// working copy after stage\n")
	drive(t, m, m.refresh())
	for _, group := range []string{"stage", "work", "stage"} {
		index := -1
		for i, row := range m.rows {
			if row.node != nil && row.group == group && row.node.Path == "file.go" {
				index = i
				break
			}
		}
		if index < 0 {
			t.Fatal("real MM path missing from group")
		}
		m.treeOffset = max(0, index-m.bodyHeight()+1)
		drive(t, m, mousePress(m, 15, m.bodyTop()+4+index-m.treeOffset))
		want := "stage"
		if group == "work" {
			want = "trabalho"
		}
		if m.viewerTab != want {
			t.Fatalf("group %s selected %s, want %s", group, m.viewerTab, want)
		}
	}
}

func TestMouse_FooterOpensPushAndWorktreesWithoutExecutingThem(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 42})
	for _, control := range []struct{ label, modal string }{{"p Push", "push"}, {"w Worktrees", "worktrees"}, {"? Ajuda", "help"}} {
		x, y := mouseLabelPosition(t, m.View(), control.label)
		drive(t, m, mousePress(m, x+1, y))
		if m.modal != control.modal || core.pushCalls != 0 || core.commitCalls != 0 {
			t.Fatalf("footer %s did not open safe preview", control.label)
		}
		press(m, "esc")
	}
}

func TestMouse_TabBordersAndPaddingShareRenderedGeometry(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	for _, size := range [][2]int{{140, 36}, {80, 24}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.focusPane = "tree"
		width := m.width
		if m.isWide {
			width, _, _ = m.paneWidths()
		}
		drive(t, m, mousePress(m, width-1, m.bodyTop()+2))
		if m.activeTab != "repositorio" {
			t.Fatal("tab bottom-right border did not select repository")
		}
		drive(t, m, mousePress(m, 0, m.bodyTop()))
		if m.activeTab != "alteracoes" {
			t.Fatal("tab top-left border did not select changes")
		}
		m.focusPane = "viewer"
		x := m.width - 1
		if m.isWide {
			left, center, _ := m.paneWidths()
			x = left + center
		}
		mousePress(m, x, m.bodyTop())
		if m.viewerTab != "arquivo" || core.stageCalls != 0 {
			t.Fatal("viewer tab padding was not read-only")
		}
	}
}

func TestFieldDisplayCursorAt_CaretWrapDoesNotShiftDraftIndex(t *testing.T) {
	if got := fieldDisplayCursorAt("abcdef", 3, 4, 1, 1, 0, true); got != 4 {
		t.Fatalf("wrapped caret map got %d want4", got)
	}
	if got := fieldDisplayCursorAt("ação", 2, 20, 0, 4, 0, true); got != 3 {
		t.Fatalf("accented caret map got %d want3", got)
	}
}

func TestMouse_ModalBodyTokensCannotActAsButtons(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	drive(t, m, press(m, "s"))
	m.modal, m.commitPhase = "commit", "review"
	m.reviewMessage = "fix: literals [Confirmar] [Editar] [Voltar] [Abrir]"
	m.commitCount = 1
	for _, token := range []string{"[Confirmar]", "[Editar]", "[Voltar]", "[Abrir]"} {
		x, y := mouseLabelPosition(t, m.View(), token)
		if cmd := mousePress(m, x+1, y); cmd != nil || m.modal != "commit" || m.mutating || core.commitCalls != 0 {
			t.Fatalf("body token %s acted as dialog button", token)
		}
	}
	box := strings.Split(m.modalView(), "\n")
	width := ansi.StringWidth(box[len(box)-4])
	x, _ := mouseLabelPosition(t, box[len(box)-4], "[Voltar]")
	x += max(0, (m.width-width)/2)
	y := max(0, (m.height-len(box))/2) + len(box) - 4
	mousePress(m, x+1, y)
	if m.modal != "" || !m.selectedNode().Status.IsStaged() {
		t.Fatal("real footer cancel failed or altered stage")
	}
}

func TestMouse_PathLiteral(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 42})
	other := filepath.Join(t.TempDir(), "[Abrir]-[Voltar]")
	if err := r.WorktreeAdd(context.Background(), dir, "feature", other, "main", false); err != nil {
		t.Fatal(err)
	}
	drive(t, m, press(m, "w"))
	x, y := mouseLabelPosition(t, m.View(), other)
	cmd := mousePress(m, x+1, y)
	if cmd != nil || m.modal != "worktrees" || m.dir != dir || m.worktrees[m.worktreeIndex].Path != other {
		t.Fatal("worktree path row closed/opened dialog or failed selection")
	}
}

func TestMouse_ResultBodyTokenIsNotAControl(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	m.modal, m.resultKind = "result", "commit"
	m.resultBody = "Commit local real · fix: literal [Voltar]"
	x, y := mouseLabelPosition(t, m.View(), "[Voltar]")
	if cmd := mousePress(m, x+1, y); cmd != nil || m.modal != "result" {
		t.Fatal("result body token dismissed the result")
	}
}

func TestMouse_LongPathRowsKeepTheirOwner(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 42})
	other := filepath.Join(t.TempDir(), strings.Repeat("long", 45))
	if err := r.WorktreeAdd(context.Background(), dir, "feature", other, "main", false); err != nil {
		t.Fatal(err)
	}
	drive(t, m, press(m, "w"))
	x, y := mouseLabelPosition(t, m.View(), strings.Repeat("long", 10))
	cmd := mousePress(m, x+1, y)
	if cmd != nil || m.modal != "worktrees" || m.dir != dir || m.worktrees[m.worktreeIndex].Path != other {
		t.Fatal("wrapped path row lost its true worktree owner or opened implicitly")
	}
}

func TestMouse_MinimumCommitPaneControlsMatchVisibleRows(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 14})
	m.focusPane = "commit"
	m.commitSummary = "fix: compact minimum"
	mousePress(m, 4, m.bodyTop()+3)
	if m.commitField != "description" || !m.commitEditing {
		t.Fatal("minimum description row did not enter editing")
	}
	if cmd := press(m, "q"); cmd != nil || m.commitDescription != "q" {
		t.Fatal("minimum description treated q as exit")
	}
	x, y := mouseLabelPosition(t, m.View(), "Tipo:")
	mousePress(m, x+1, y)
	if m.modal != "suggest" || core.commitCalls != 0 {
		t.Fatal("minimum type row opened review or executed commit")
	}
	press(m, "esc")
	x, y = mouseLabelPosition(t, m.View(), "[c] Revisar commit local")
	drive(t, m, mousePress(m, x+1, y))
	if m.modal != "commit" || m.commitPhase != "review" || core.commitCalls != 0 {
		t.Fatal("visible minimum review row did not open review")
	}
	x, y = mouseLabelPosition(t, m.View(), "[Voltar]")
	mousePress(m, x+1, y)
	if m.modal != "" || m.commitSummary != "fix: compact minimum" || m.commitDescription != "q" || !m.selectedNode().Status.IsStaged() {
		t.Fatal("minimum review cancel lost stage or draft")
	}
}
