package tui

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type observedService struct {
	app.Service
	stageCalls, commitCalls, pushCalls int
}

func (s *observedService) ToggleStage(ctx context.Context, dir, path string) error {
	s.stageCalls++
	return s.Service.ToggleStage(ctx, dir, path)
}
func (s *observedService) CreateCommit(ctx context.Context, dir, message string) (*domain.CommitResult, error) {
	s.commitCalls++
	return s.Service.CreateCommit(ctx, dir, message)
}
func (s *observedService) PushChanges(ctx context.Context, dir string, opts domain.PushOptions) (*domain.PushResult, error) {
	s.pushCalls++
	return s.Service.PushChanges(ctx, dir, opts)
}

func setupStudio(t *testing.T) (*Model, *observedService, *git.NativeRunner, string, string) {
	t.Helper()
	r, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	dir, remote := t.TempDir(), t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Studio Test"}, {"config", "user.email", "studio@example.com"}, {"config", "commit.gpgsign", "false"}, {"config", "core.hooksPath", filepath.Join(t.TempDir(), "hooks")}, {"remote", "add", "origin", remote}} {
		if _, err := r.Exec(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Exec(ctx, remote, "init", "--bare", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	writeStudio(t, dir, "file.go", "package main\n")
	writeStudio(t, dir, "nested/clean.md", "# Workspace\n")
	if err := r.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "chore: initial"); err != nil {
		t.Fatal(err)
	}
	writeStudio(t, dir, "file.go", "package main\n\n// changed\n")
	core := &observedService{Service: app.NewApp(r)}
	m := NewModel(ctx, core, dir, false)
	drive(t, m, m.refresh())
	for i, row := range m.rows {
		if row.node.Path == "file.go" {
			m.selected = i
		}
	}
	drive(t, m, m.loadSelected())
	return m, core, r, dir, remote
}

func writeStudio(t *testing.T, dir, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func drive(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	for i := 0; cmd != nil && i < 10; i++ {
		msg := cmd()
		if _, ok := msg.(tea.BatchMsg); ok {
			t.Fatal("unexpected batch in synchronous test driver")
		}
		_, cmd = m.Update(msg)
	}
	if cmd != nil {
		t.Fatal("command chain did not settle")
	}
}

func press(m *Model, s string) tea.Cmd {
	keys := map[string]tea.KeyType{"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "down": tea.KeyDown, "up": tea.KeyUp, "ctrl+u": tea.KeyCtrlU, "ctrl+c": tea.KeyCtrlC}
	if key, ok := keys[s]; ok {
		_, cmd := m.Update(tea.KeyMsg{Type: key})
		return cmd
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return cmd
}

func TestModel_StageCommitAndViewport(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	if NewModel(context.Background(), core, dir, false).Init() == nil {
		t.Fatal("missing initial commands")
	}
	if m.viewer.mode != "diff" {
		t.Fatalf("expected diff, got %s", m.viewer.mode)
	}
	press(m, "tab")
	if m.viewer.mode != "arquivo" || !strings.Contains(ansi.Strip(strings.Join(m.viewer.lines, "\n")), "1 │ package main") {
		t.Fatal("file viewport missing numbered content")
	}
	press(m, "shift+tab")
	if m.focus != "viewer" {
		t.Fatal("focus did not change")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 120 {
			t.Fatalf("overflow: %d", ansi.StringWidth(line))
		}
	}
	if len(strings.Split(view, "\n")) != 35 {
		t.Fatalf("height %d", len(strings.Split(view, "\n")))
	}
	cmd := press(m, "s")
	if cmd == nil || core.stageCalls != 0 {
		t.Fatal("stage ran inside Update instead of tea.Cmd")
	}
	drive(t, m, cmd)
	if core.stageCalls != 1 || !m.selectedNode().Status.IsStaged() {
		t.Fatal("stage did not refresh tree")
	}
	press(m, "c")
	if m.modal != "commit" || m.commitPhase != "type" {
		t.Fatal("commit type modal missing")
	}
	drive(t, m, press(m, "1"))
	if m.commitPhase != "review" || !strings.HasPrefix(m.commitMessage, "feat:") {
		t.Fatalf("proposal %q phase %q", m.commitMessage, m.commitPhase)
	}
	press(m, "e")
	press(m, "ctrl+u")
	press(m, "fix: edited in studio")
	press(m, "enter")
	if m.commitPhase != "review" || m.commitMessage != "fix: edited in studio" {
		t.Fatal("message editor failed")
	}
	cmd = press(m, "enter")
	if core.commitCalls != 0 || cmd == nil {
		t.Fatal("commit ran inside Update")
	}
	drive(t, m, cmd)
	if core.commitCalls != 1 || core.pushCalls != 0 {
		t.Fatal("commit implicitly pushed")
	}
	out, err := r.Exec(context.Background(), dir, "log", "-1", "--format=%s")
	if err != nil || strings.TrimSpace(out) != "fix: edited in studio" {
		t.Fatalf("commit %q: %v", out, err)
	}
}

func TestModel_PushRequiresExplicitConfirmation(t *testing.T) {
	m, core, r, dir, remote := setupStudio(t)
	drive(t, m, press(m, "p"))
	if core.pushCalls != 0 || m.pushTarget == nil || !strings.Contains(ansi.Strip(m.View()), "Local: main -> Remote: origin/main") {
		t.Fatal("push preview missing or performed push")
	}
	press(m, "esc")
	if core.pushCalls != 0 || m.modal != "" {
		t.Fatal("cancel performed push")
	}
	drive(t, m, press(m, "p"))
	cmd := press(m, "enter")
	if core.pushCalls != 0 || cmd == nil {
		t.Fatal("push ran synchronously")
	}
	drive(t, m, cmd)
	if core.pushCalls != 1 {
		t.Fatal("confirmed push was not executed")
	}
	head, err := r.RevParseHead(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	remoteHead, err := r.RevParseHead(context.Background(), remote)
	if err != nil || remoteHead != head {
		t.Fatalf("publication mismatch %s: %v", remoteHead, err)
	}
}

func TestModel_WorktreeSwitchAndStaleReads(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	other := filepath.Join(t.TempDir(), "linked")
	if err := r.WorktreeAdd(context.Background(), dir, "feature", other, "main", false); err != nil {
		t.Fatal(err)
	}
	drive(t, m, press(m, "w"))
	for i, w := range m.worktrees {
		if w.Path == other {
			m.worktreeIndex = i
		}
	}
	oldID := m.loadID
	drive(t, m, press(m, "enter"))
	if m.dir != other || m.studio.Workspace.Branch != "feature" {
		t.Fatal("workspace was not switched")
	}
	m.Update(treeMsg{id: oldID, tree: &domain.WorkspaceTree{Workspace: domain.Workspace{Path: dir}}})
	if m.dir != other {
		t.Fatal("stale tree replaced current workspace")
	}
	currentPath := m.viewer.path
	m.Update(fileMsg{id: m.fileID - 1, path: "stale"})
	if m.viewer.path != currentPath {
		t.Fatal("stale file replaced viewer")
	}
}

func TestModel_RefreshPreservesViewportAndCancelStage(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	writeStudio(t, m.dir, "file.go", strings.Repeat("// line\n", 100))
	drive(t, m, m.refresh())
	press(m, "tab")
	m.viewer.offset = 20
	m.viewer.column = 3
	drive(t, m, m.refresh())
	if m.viewer.mode != "arquivo" || m.viewer.offset != 20 || m.viewer.column != 3 {
		t.Fatal("refresh lost reading position")
	}
	drive(t, m, press(m, "s"))
	press(m, "c")
	press(m, "esc")
	if !m.selectedNode().Status.IsStaged() || core.commitCalls != 0 {
		t.Fatal("cancel changed the stage")
	}
	press(m, "?")
	if !strings.Contains(ansi.Strip(m.View()), "ATALHOS") {
		t.Fatal("help overlay missing")
	}
	press(m, "esc")
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	if !strings.Contains(m.View(), "Amplie") {
		t.Fatal("small-terminal fallback missing")
	}
}

func TestSafeTextAndIcons(t *testing.T) {
	if strings.Contains(safeText("hello\x1b]52;c;payload\a"), "\x1b") {
		t.Fatal("repository text can inject terminal controls")
	}
	node := &domain.TreeNode{Name: "main.go"}
	if fileIcon(node, false, false) != "[go]" || fileIcon(node, false, true) == "[go]" {
		t.Fatal("font fallback missing")
	}
}

func TestModel_ResponsiveOverlays(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	for _, size := range [][2]int{{50, 14}, {80, 24}, {120, 36}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, modal := range []string{"", "help", "commit", "push", "worktrees"} {
			m.modal = modal
			m.commitPhase = "type"
			lines := strings.Split(m.View(), "\n")
			if len(lines) > size[1] {
				t.Fatalf("%s at %v overflows height: %d", modal, size, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%s at %v overflows width: %d", modal, size, ansi.StringWidth(line))
				}
			}
		}
	}
}

func TestStudio_ProgramLifecycle(t *testing.T) {
	m, core, _, dir, _ := setupStudio(t)
	_ = m
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := Run(ctx, core, dir, strings.NewReader("q"), &out, false); err != nil {
		t.Fatalf("program shutdown: %v", err)
	}
	if !strings.Contains(out.String(), "\x1b[?1049h") || !strings.Contains(out.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen not restored")
	}
}
