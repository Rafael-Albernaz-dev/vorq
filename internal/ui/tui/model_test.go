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
	"github.com/charmbracelet/lipgloss"
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
func (s *observedService) CreateReviewedCommit(ctx context.Context, dir, message, diff string) (*domain.CommitResult, error) {
	s.commitCalls++
	return s.Service.CreateReviewedCommit(ctx, dir, message, diff)
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
	for path, text := range map[string]string{"file.go": "package main\n", "nested/clean.md": "# Workspace\n", ".gitignore": ".env\nbin/\nnode_modules/\n", ".github/workflows/check.yml": "name: check\n", "config.json": "{}\n", "deleted.txt": "delete me\n"} {
		writeStudio(t, dir, path, text)
	}
	if err := r.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "chore: initial"); err != nil {
		t.Fatal(err)
	}
	writeStudio(t, dir, "file.go", "package main\n\n// changed\n")
	writeStudio(t, dir, ".env", "private\n")
	writeStudio(t, dir, "bin/ignored", "ignored\n")
	core := &observedService{Service: app.NewApp(git.NewRepository(r))}
	m := NewModel(ctx, core, dir)
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
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
			t.Fatal("unexpected batch in test driver")
		}
		_, cmd = m.Update(msg)
	}
	if cmd != nil {
		t.Fatal("command chain did not settle")
	}
}
func press(m *Model, s string) tea.Cmd {
	keys := map[string]tea.KeyType{"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "down": tea.KeyDown, "up": tea.KeyUp, "left": tea.KeyLeft, "right": tea.KeyRight, "pgdown": tea.KeyPgDown, "pgup": tea.KeyPgUp, "ctrl+u": tea.KeyCtrlU, "ctrl+c": tea.KeyCtrlC, "ctrl+g": tea.KeyCtrlG, "ctrl+s": tea.KeyCtrlS}
	if key, ok := keys[s]; ok {
		_, cmd := m.Update(tea.KeyMsg{Type: key})
		return cmd
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return cmd
}
func selectPath(t *testing.T, m *Model, path string) {
	t.Helper()
	for i, row := range m.rows {
		if row.node != nil && row.node.Path == path {
			m.selected = i
			drive(t, m, m.loadSelected())
			return
		}
	}
	t.Fatalf("path not visible in %s: %s", m.activeTab, path)
}
func assertFrame(t *testing.T, m *Model) {
	t.Helper()
	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("height %d != %d", len(lines), m.height)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) != m.width {
			t.Fatalf("width %d != %d: %q", ansi.StringWidth(line), m.width, ansi.Strip(line))
		}
	}
}

func TestModel_TabsInventoryAndCleanFile(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	if m.activeTab != "alteracoes" || m.changeSummary().Total != 1 || m.inventoryCount != 6 {
		t.Fatalf("inventory %d, summary %+v", m.inventoryCount, m.changeSummary())
	}
	for _, row := range m.rows {
		if row.node != nil && row.node.Path == "nested/clean.md" {
			t.Fatal("clean file leaked into changes")
		}
	}
	drive(t, m, press(m, "e"))
	if m.activeTab != "repositorio" || m.selectedNode().Path != "file.go" {
		t.Fatal("tab switch lost selection")
	}
	seen := map[string]bool{}
	for _, row := range m.rows {
		if row.node != nil {
			seen[row.node.Path] = true
		}
	}
	if !seen[".github"] || !seen[".gitignore"] || seen[".git"] || seen[".env"] || seen["bin"] {
		t.Fatalf("inventory visibility: %v", seen)
	}
	selectPath(t, m, "config.json")
	if m.viewerTab != "arquivo" {
		t.Fatal("clean file did not open full content")
	}
	if cmd := press(m, "s"); cmd != nil || core.stageCalls != 0 || m.notice != "Arquivo sem alterações" {
		t.Fatal("clean-file stage was not disabled")
	}
	drive(t, m, press(m, "a"))
	if m.activeTab != "alteracoes" {
		t.Fatal("changes tab not restored")
	}
}

func TestModel_ViewerTabsAndSeparateDiffs(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	if m.viewerTab != "trabalho" {
		t.Fatal("worktree diff not selected")
	}
	drive(t, m, press(m, "s"))
	writeStudio(t, dir, "file.go", "package main\n\n// after stage\n")
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	press(m, "1")
	work := ansi.Strip(strings.Join(m.viewer.lines, "\n"))
	if !strings.Contains(work, "after stage") {
		t.Fatalf("work diff: %s", work)
	}
	press(m, "2")
	stage := ansi.Strip(strings.Join(m.viewer.lines, "\n"))
	if strings.Contains(stage, "after stage") || !strings.Contains(stage, "changed") {
		t.Fatalf("stage diff: %s", stage)
	}
	press(m, "tab")
	if m.viewerTab != "arquivo" || !strings.Contains(ansi.Strip(strings.Join(m.viewer.lines, "\n")), "1 | package main") {
		t.Fatal("numbered full file missing")
	}
	press(m, "tab")
	if m.viewerTab != "stage" {
		t.Fatal("Tab did not restore previous diff tab")
	}
	if !strings.Contains(stage, "3 + // changed") {
		t.Fatalf("missing added-line number: %s", stage)
	}
}

func TestModel_StageCommitMultilineAndNoPush(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	if NewModel(context.Background(), core, dir).Init() == nil {
		t.Fatal("missing initial commands")
	}
	cmd := press(m, "s")
	if cmd == nil || core.stageCalls != 0 {
		t.Fatal("I/O ran inside Update")
	}
	drive(t, m, cmd)
	if core.stageCalls != 1 || m.changeSummary().Staged != 1 {
		t.Fatal("stage not refreshed")
	}
	press(m, "c")
	if m.focusPane != "commit" || !m.commitEditing {
		t.Fatal("commit editor not focused")
	}
	press(m, "fix: edited in vorq")
	press(m, "enter")
	press(m, "Descrição real")
	press(m, "enter")
	press(m, "Segunda linha")
	if m.commitDescription != "Descrição real\nSegunda linha" {
		t.Fatalf("body %q", m.commitDescription)
	}
	drive(t, m, press(m, "ctrl+s"))
	if m.modal != "commit" || m.commitPhase != "review" || core.commitCalls != 0 {
		t.Fatal("review missing or committed without confirmation")
	}
	cmd = press(m, "enter")
	if cmd == nil || core.commitCalls != 0 {
		t.Fatal("commit ran inside Update")
	}
	drive(t, m, cmd)
	if core.commitCalls != 1 || core.pushCalls != 0 {
		t.Fatal("commit implicitly pushed")
	}
	out, err := r.Exec(context.Background(), dir, "log", "-1", "--format=%B")
	if err != nil || strings.TrimSpace(out) != "fix: edited in vorq\n\nDescrição real\nSegunda linha" {
		t.Fatalf("message %q: %v", out, err)
	}
	if m.commitSummary != "" || m.commitDescription != "" {
		t.Fatal("successful commit did not clear submitted draft")
	}
}

func TestModel_EditorConsumesAllPrintableShortcuts(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	press(m, "c")
	for _, s := range []string{"a", "e", "s", "c", "q", "p", "w", "i", "r", "?", "1", "2"} {
		if cmd := press(m, s); cmd != nil {
			t.Fatalf("shortcut %q escaped editor", s)
		}
	}
	if m.commitSummary != "aescqpwir?12" || m.activeTab != "alteracoes" || m.modal != "" || core.stageCalls != 0 || core.pushCalls != 0 {
		t.Fatalf("summary %q, unexpected global state", m.commitSummary)
	}
	press(m, "enter")
	for _, s := range []string{"q", "s", "a", "e"} {
		press(m, s)
	}
	if m.commitDescription != "qsae" {
		t.Fatal("description treated letters as global keys")
	}
	press(m, "esc")
	if m.commitEditing || m.focusPane != "commit" {
		t.Fatal("Esc did not exit field editing")
	}
	press(m, "esc")
	if m.focusPane != "tree" {
		t.Fatal("Esc did not return to navigation")
	}
}

func TestModel_SuggestionPreservesBodyAndRejectsOldResponses(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	m.commitDescription = "Não substituir a descrição"
	press(m, "c")
	press(m, "ctrl+g")
	if m.modal != "suggest" {
		t.Fatal("suggestion shortcut missing in editor")
	}
	drive(t, m, press(m, "1"))
	if !strings.HasPrefix(m.commitSummary, "feat:") || m.commitDescription != "Não substituir a descrição" {
		t.Fatal("suggestion failed or replaced description")
	}
	press(m, "ctrl+g")
	cmd := press(m, "2")
	press(m, "esc")
	draft := m.commitSummary
	drive(t, m, cmd)
	if m.commitSummary != draft || m.busy != "" {
		t.Fatal("cancelled suggestion updated the draft")
	}
	m.Update(proposalMsg{id: m.dialogID - 1})
	if m.commitSummary != draft {
		t.Fatal("stale suggestion changed draft")
	}
}

func TestModel_UnionCountersAndBothMarker(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	writeStudio(t, dir, "file.go", "package main\n// after stage\n")
	writeStudio(t, dir, "nested/new.txt", "new\n")
	if err := os.Remove(filepath.Join(dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	drive(t, m, m.refresh())
	s := m.changeSummary()
	if s.Total != 3 || s.Staged != 1 || s.Unstaged != 2 || s.Untracked != 1 || s.Both != 1 {
		t.Fatalf("union counts %+v", s)
	}
	selectPath(t, m, "file.go")
	if ansi.Strip(stageMarker(m.selectedNode().Status)) != "[-]" {
		t.Fatal("partial stage marker missing")
	}
	headings := map[string]bool{}
	for _, row := range m.rows {
		if row.node == nil {
			headings[row.group] = true
		}
	}
	if !headings["stage"] || !headings["work"] || !headings["new"] {
		t.Fatal("change groups missing")
	}
	if s.Total >= m.inventoryCount {
		t.Fatal("clean files inflated change union")
	}
}

func TestModel_ResponsiveLayoutPreservesDraftAndScroll(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	writeStudio(t, dir, "file.go", strings.Repeat("// line\n", 100))
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	press(m, "tab")
	m.viewer.offset = 20
	m.viewer.column = 3
	m.commitSummary = "rascunho"
	m.commitDescription = "linha 1\nlinha 2"
	for _, size := range [][2]int{{140, 36}, {139, 36}, {140, 35}, {80, 24}, {180, 45}, {50, 14}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		wide := size[0] >= 140 && size[1] >= 36
		if m.isWide != wide {
			t.Fatalf("wrong breakpoint %v", size)
		}
		for _, pane := range []string{"tree", "viewer", "commit"} {
			m.focusPane = pane
			assertFrame(t, m)
		}
		if m.selectedNode().Path != "file.go" || m.viewer.offset != 20 || m.viewer.column != 3 || m.commitSummary != "rascunho" || m.commitDescription != "linha 1\nlinha 2" {
			t.Fatal("resize discarded state")
		}
		if wide {
			left, center, right := m.paneWidths()
			available := size[0] - 2
			if left != available*25/100 || right != available*31/100 || left+center+right+2 != size[0] {
				t.Fatal("wrong 25/44/31 geometry")
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.focusPane = "tree"
	press(m, "shift+tab")
	if m.focusPane != "viewer" {
		t.Fatal("viewer focus missing")
	}
	press(m, "shift+tab")
	if m.focusPane != "commit" {
		t.Fatal("third focus pane missing")
	}
	press(m, "shift+tab")
	if m.focusPane != "tree" {
		t.Fatal("focus cycle not closed")
	}
	m.Update(tea.WindowSizeMsg{Width: 49, Height: 14})
	if m.View() != "vorq · Workspace Studio\nAmplie o terminal para pelo menos 50×14.\nq / Ctrl+C para sair." {
		t.Fatal("minimum-size contract changed")
	}
}

func TestModel_RefreshPreservesViewportAndCancelReview(t *testing.T) {
	m, core, _, dir, _ := setupStudio(t)
	writeStudio(t, dir, "file.go", strings.Repeat("// line\n", 100))
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	press(m, "tab")
	m.viewer.offset = 20
	m.viewer.column = 3
	drive(t, m, m.refresh())
	if m.viewerTab != "arquivo" || m.viewer.offset != 20 || m.viewer.column != 3 {
		t.Fatal("refresh lost reading position")
	}
	drive(t, m, press(m, "s"))
	m.commitSummary = "fix: review"
	drive(t, m, press(m, "ctrl+s"))
	press(m, "esc")
	if m.changeSummary().Staged != 1 || core.commitCalls != 0 {
		t.Fatal("cancel changed stage")
	}
}

func TestModel_HookFailureIsContextualAndPreservesDraft(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	hooks := t.TempDir()
	if _, err := r.Exec(context.Background(), dir, "config", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho rejection-from-real-hook >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	m.commitSummary = "fix: preserve draft"
	m.commitDescription = "body"
	before, err := r.RevParseHead(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	drive(t, m, press(m, "ctrl+s"))
	drive(t, m, press(m, "enter"))
	after, err := r.RevParseHead(context.Background(), dir)
	if err != nil || before != after || core.commitCalls != 1 || m.modal != "error" || !strings.Contains(m.lastError, "rejection-from-real-hook") || m.commitSummary != "fix: preserve draft" || m.changeSummary().Staged != 1 {
		t.Fatalf("hook error handling failed: %s", m.lastError)
	}
}

func TestModel_PushRequiresExplicitConfirmation(t *testing.T) {
	m, core, r, dir, remote := setupStudio(t)
	drive(t, m, press(m, "p"))
	if core.pushCalls != 0 || m.pushTarget == nil || !strings.Contains(ansi.Strip(m.View()), "Local: main -> Remote: origin/main") {
		t.Fatal("push preview missing or push performed")
	}
	press(m, "esc")
	if core.pushCalls != 0 {
		t.Fatal("cancel performed push")
	}
	drive(t, m, press(m, "p"))
	cmd := press(m, "enter")
	if core.pushCalls != 0 || cmd == nil {
		t.Fatal("push ran in Update")
	}
	drive(t, m, cmd)
	head, err := r.RevParseHead(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	remoteHead, err := r.RevParseHead(context.Background(), remote)
	if err != nil || remoteHead != head || core.pushCalls != 1 {
		t.Fatal("confirmed publication mismatch")
	}
}
func TestModel_WorktreeSwitchAndStaleReads(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	m.commitSummary = "draft in original"
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
	if m.dir != other || m.studio.Workspace.Branch != "feature" || m.commitSummary != "" {
		t.Fatal("workspace did not switch or leaked draft")
	}
	m.Update(treeMsg{id: oldID, tree: &domain.WorkspaceTree{Workspace: domain.Workspace{Path: dir}}})
	if m.dir != other {
		t.Fatal("stale tree installed")
	}
	currentPath := m.viewer.path
	m.Update(fileMsg{id: m.fileID - 1, path: "stale"})
	if m.viewer.path != currentPath {
		t.Fatal("stale file installed")
	}
	drive(t, m, m.switchWorkspace(dir))
	if m.commitSummary != "draft in original" {
		t.Fatal("workspace draft not restored")
	}
}

func TestModel_SpecialFilesAreRealAndReadOnly(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	writeStudio(t, dir, "binary.dat", "prefix\x00suffix")
	if err := os.Symlink("nested/clean.md", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	drive(t, m, m.refresh())
	selectPath(t, m, "binary.dat")
	if !m.viewer.content.Binary || strings.Contains(ansi.Strip(strings.Join(m.viewer.lines, "\n")), "1 |") {
		t.Fatal("binary invented text lines")
	}
	selectPath(t, m, "link")
	if !strings.Contains(m.viewer.content.Notice, "Link simbólico") {
		t.Fatal("symlink not identified")
	}
	selectPath(t, m, "deleted.txt")
	if !strings.Contains(m.viewer.diff.Unstaged, "-delete me") || !strings.Contains(m.viewer.content.Notice, "Arquivo excluído") {
		t.Fatal("deleted diff lost")
	}
}
func TestSafeTextPaletteAndIcons(t *testing.T) {
	if strings.Contains(safeText("hello\x1b]52;c;payload\a"), "\x1b") {
		t.Fatal("terminal injection")
	}
	for name, want := range map[string]string{"main.go": "[GO]", "README.md": "[MD]", "config.json": "[JSON]", ".env.example": "[ENV]"} {
		node := &domain.TreeNode{Name: name}
		if got := fileIcon(node, false); got != want {
			t.Fatalf("icon %q != %q", got, want)
		}
	}
	palette := map[lipgloss.Color]string{ColorBg: "#1a1b26", ColorPanel: "#16161e", ColorRaised: "#24283b", ColorText: "#c0caf5", ColorMuted: "#9aa5ce", ColorLine: "#414868", ColorAccent: "#7aa2f7", ColorSelection: "#283457", ColorGreen: "#9ece6a", ColorRed: "#f7768e", ColorYellow: "#e0af68", ColorPurple: "#bb9af7", ColorBar: "#2e4270", ColorBarText: "#ffffff", ColorCyan: "#7dcfff"}
	if len(palette) != 15 {
		t.Fatal("palette tokens missing")
	}
	for token, want := range palette {
		if string(token) != want {
			t.Fatalf("token %s != %s", token, want)
		}
	}
}
func TestModel_ResponsiveOverlays(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	for _, size := range [][2]int{{50, 14}, {80, 24}, {140, 36}, {180, 45}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, modal := range []string{"", "help", "commit", "suggest", "push", "worktrees", "error"} {
			m.modal = modal
			m.commitPhase = "review"
			assertFrame(t, m)
		}
	}
}
func TestStudio_ProgramLifecycle(t *testing.T) {
	_, core, _, dir, _ := setupStudio(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var out bytes.Buffer
	if err := Run(ctx, core, dir, strings.NewReader("q"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1b[?1049h") || !strings.Contains(out.String(), "\x1b[?1049l") {
		t.Fatal("alternate screen not restored")
	}
}

func TestModel_ConflictGroupComesFromNativeGit(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	if err := core.StageFiles(ctx, dir, []string{"file.go"}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Service.CreateCommit(ctx, dir, "chore: base"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exec(ctx, dir, "checkout", "-b", "side"); err != nil {
		t.Fatal(err)
	}
	writeStudio(t, dir, "file.go", "side version\n")
	if err := r.Add(ctx, dir, "file.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "fix: side"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exec(ctx, dir, "checkout", "main"); err != nil {
		t.Fatal(err)
	}
	writeStudio(t, dir, "file.go", "main version\n")
	if err := r.Add(ctx, dir, "file.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "fix: main"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exec(ctx, dir, "merge", "side"); err == nil {
		t.Fatal("expected a real merge conflict")
	}
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	s := m.changeSummary()
	if s.Total != 1 || s.Conflicts != 1 || s.Staged != 0 || s.Unstaged != 0 {
		t.Fatalf("conflict counts %+v", s)
	}
	if ansi.Strip(statusBadge(m.selectedNode().Status)) != "U" {
		t.Fatal("native unmerged badge missing")
	}
	if m.viewerTab != "trabalho" || m.viewer.diff.Unstaged == "" {
		t.Fatal("conflict diff unavailable")
	}
	found := false
	for _, row := range m.rows {
		if row.group == "conflict" && row.node == nil {
			found = true
		}
	}
	if !found {
		t.Fatal("conflict group missing")
	}
}
