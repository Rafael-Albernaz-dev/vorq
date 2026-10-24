package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestIconsSafeModesCellWidthsAndCycling(t *testing.T) {
	names := []string{"f.go", "a.ts", "a.tsx", "a.js", "a.jsx", "a.py", "a.html", "a.css", "a.md", "a.json", "a.yaml", "a.yml", "a.toml", ".env", "a.ini", ".gitignore", ".gitattributes", "go.mod", "go.sum", "a.png", "a.jpg", "a.svg", "a.sh", "a.bash", "unknown"}
	for _, mode := range []IconMode{IconModeEmoji, IconModeText} {
		for _, name := range names {
			icon := nodeIcon(&domain.TreeNode{Name: name}, false, mode)
			if icon == "" {
				t.Fatal("empty icon")
			}
			if mode == IconModeEmoji && ansi.StringWidth(icon) != 2 {
				t.Errorf("%s: emoji %q width %d", name, icon, ansi.StringWidth(icon))
			}
			for _, r := range icon {
				if unicode.Is(unicode.Co, r) {
					t.Errorf("PUA in safe mode: %q", icon)
				}
				if mode == IconModeText && r > 127 {
					t.Errorf("non ASCII badge: %q", icon)
				}
			}
		}
		for _, open := range []bool{false, true} {
			if got := nodeIcon(&domain.TreeNode{IsDir: true}, open, mode); mode == IconModeEmoji && ansi.StringWidth(got) != 2 {
				t.Fatal("directory width")
			}
		}
	}
	m := NewModel(context.Background(), nil, "/tmp")
	for _, want := range []IconMode{IconModeEmoji, IconModeText, IconModeEmoji} {
		if m.currentIconMode() != want {
			t.Fatalf("mode %s want %s", m.currentIconMode(), want)
		}
		press(m, "i")
	}
	if _, err := ParseIconMode("bad"); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func color256(t *testing.T) {
	t.Helper()
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
}

func TestHighlightCodeLinesAndSnippetPreserveSource(t *testing.T) {
	color256(t)
	source := "package main\n\n/* multi\nline comment */\nfunc main() { println(\"ação 🔷\") }\n"
	for _, palette := range studioPalettes {
		if styles.Registry[chromaTheme(palette.name)] == nil {
			t.Fatalf("missing theme %s", chromaTheme(palette.name))
		}
		lines := highlightCodeLines("main.go", source, palette.name)
		if len(lines) != 5 {
			t.Fatalf("line count %d", len(lines))
		}
		got := highlightCodeSnippet("main.go", source, palette.name)
		if ansi.Strip(got) != strings.TrimSuffix(source, "\n") {
			t.Fatalf("source altered: %q", ansi.Strip(got))
		}
		if !strings.Contains(got, "\x1b[39;38;5;") {
			t.Fatalf("no terminal256 highlight for %s", palette.name)
		}
		if strings.Contains(got, "\x1b[48;") {
			t.Fatal("syntax background masks diff tint")
		}
		m := NewModel(context.Background(), nil, "/tmp")
		for i, p := range studioPalettes {
			if p.name == palette.name {
				m.themeIndex = i
			}
		}
		if m.applyTheme(got) != got {
			t.Fatalf("chrome remapped %s syntax colors", palette.name)
		}
		if !strings.Contains(lines[3], "\x1b[") {
			t.Fatal("multiline comment lost highlighting")
		}
	}
	for _, source := range []string{"", "\n", "a\n\n", "#!/usr/bin/env python\nprint('ok')\n", "literal \x1b[31mred\tend"} {
		got := highlightCodeSnippet("unknown.filetype", source, "Nord")
		if ansi.Strip(got) != strings.TrimSuffix(safeText(source), "\n") {
			t.Fatalf("fallback/control handling changed source: %q", got)
		}
	}
	for _, theme := range []string{"Papel claro", "Alto contraste"} {
		if got := highlightCodeSnippet("f.go", "package main\n☃ !!! `unfinished", theme); strings.Contains(got, "\x1b[48;") {
			t.Fatal("invalid code hid diff backgrounds")
		}
	}
}

func TestViewerHighlightGuttersDiffBackgroundsAndTheme(t *testing.T) {
	color256(t)
	v := viewerState{path: "f.go", mode: "arquivo", theme: "Nord", content: &domain.FileContent{Text: "package main\nfunc main() {}\n"}}
	v.rebuild()
	first := strings.Join(v.lines, "\n")
	if !strings.HasPrefix(ansi.Strip(v.lines[0]), "   1 | package main") {
		t.Fatal("file gutter lost")
	}
	v.theme = "Gruvbox"
	v.rebuild()
	if first == strings.Join(v.lines, "\n") {
		t.Fatal("theme did not change syntax")
	}
	v.diff = &domain.FileDiff{Unstaged: "diff --git a/f.go b/f.go\n@@ -1,2 +1,2 @@\n package main\n-func old() {}\n+func newName() {}\n"}
	v.mode = "trabalho"
	v.rebuild()
	if v.added != 1 || v.deleted != 1 {
		t.Fatal("diff counters changed")
	}
	if v.lineBackgrounds[2] != ColorDeletedBg || v.lineBackgrounds[3] != ColorAddedBg {
		t.Fatal("diff tints lost")
	}
	for _, row := range []int{2, 3} {
		if !strings.Contains(v.lines[row], "\x1b[39;38;5;") {
			t.Fatal("diff code lacks Chroma")
		}
	}
	for _, mode := range []IconMode{IconModeEmoji, IconModeText} {
		m, _, _, _, _ := setupStudio(t)
		m.SetIconMode(mode)
		for _, size := range [][2]int{{50, 14}, {80, 24}, {140, 36}, {180, 42}} {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			assertFrame(t, m)
		}
	}
}

func TestViewerCachesModesInvalidatesOnContentAndHonorsCancellation(t *testing.T) {
	color256(t)
	v := viewerState{path: "f.go", mode: "arquivo", theme: "Nord", content: &domain.FileContent{Text: "package main\n"}}
	if err := v.prepareModes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(v.renderCache) != 3 {
		t.Fatal("all three modes must be ready before delivery")
	}
	original := &v.lines[0]
	v.setMode("trabalho")
	v.setMode("arquivo")
	if original != &v.lines[0] {
		t.Fatal("tab switch recomputed identical source")
	}
	// Refresh returns new pointers for the same bytes; the cache must survive.
	v.content = &domain.FileContent{Text: "package main\n"}
	if err := v.prepareModes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if original != &v.lines[0] {
		t.Fatal("unchanged refresh invalidated cache")
	}
	v.content = &domain.FileContent{Text: "package changed\n"}
	if err := v.prepareModes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ansi.Strip(v.lines[0]), "changed") {
		t.Fatal("changed source reused stale cache")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.prepareModes(ctx); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestBatchStageUnstageAndEmptyNotices(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	writeStudio(t, dir, "new.txt", "new\n")
	drive(t, m, m.refresh())
	drive(t, m, press(m, "A"))
	if s := m.changeSummary(); s.Staged != 2 || s.Untracked != 0 || s.Unstaged != 0 {
		t.Fatalf("stage all: %+v", s)
	}
	if cmd := press(m, "A"); cmd != nil || !strings.Contains(m.notice, "Nenhuma alteração") {
		t.Fatal("empty stage-all lacks notice")
	}
	drive(t, m, press(m, "U"))
	if s := m.changeSummary(); s.Staged != 0 || s.Untracked != 1 || s.Unstaged != 1 {
		t.Fatalf("unstage all: %+v", s)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "new.txt")); err != nil || string(data) != "new\n" {
		t.Fatal("unstage lost data")
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
		t.Fatal("batch affected ignored file")
	}
	if cmd := press(m, "U"); cmd != nil || !strings.Contains(m.notice, "Nenhum arquivo") {
		t.Fatal("empty unstage-all lacks notice")
	}
}

func TestNewActionsBlockedAndEditingWins(t *testing.T) {
	for _, state := range []string{"loading", "file-loading", "busy", "mutating", "stale"} {
		m, _, _, _, _ := setupStudio(t)
		switch state {
		case "loading":
			m.loading = true
		case "file-loading":
			m.viewer.loading = true
		case "busy":
			m.busy = "read"
		case "mutating":
			m.mutating = true
		case "stale":
			m.stale = true
		}
		for _, key := range []string{"A", "U", "x", "d"} {
			if cmd := press(m, key); cmd != nil {
				t.Fatalf("%s allowed while %s", key, state)
			}
		}
	}
	m, _, _, _, _ := setupStudio(t)
	m.focusPane = "commit"
	m.commitEditing = true
	for _, key := range []string{"A", "U", "x", "d"} {
		if cmd := press(m, key); cmd != nil {
			t.Fatal("editing triggered action")
		}
	}
	if m.commitSummary != "AUxd" || m.modal != "" {
		t.Fatal("global shortcuts stole editor text")
	}
}

func TestDiscardModalKeyboardMouseCancelAndPreserve(t *testing.T) {
	for _, action := range []string{"keyboard-delete", "mouse-delete", "keyboard-preserve", "mouse-preserve", "cancel"} {
		t.Run(action, func(t *testing.T) {
			m, _, r, dir, _ := setupStudio(t)
			ctx := context.Background()
			m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
			writeStudio(t, dir, "new.go", "new code\n")
			if err := r.Add(ctx, dir, "new.go"); err != nil {
				t.Fatal(err)
			}
			drive(t, m, m.refresh())
			selectPath(t, m, "new.go")
			x, y := mouseLabelPosition(t, m.View(), "[Descartar]")
			cmd := mousePress(m, x, y)
			if m.mutating || m.modal != "discard" {
				t.Fatal("click silently discarded")
			}
			drive(t, m, cmd)
			if m.discardPlan == nil || !strings.Contains(m.modalView(), "permanentemente") {
				t.Fatal("impact missing")
			}
			if _, err := os.Stat(filepath.Join(dir, "new.go")); err != nil {
				t.Fatal("inspection deleted file")
			}
			if strings.HasPrefix(action, "mouse-") {
				label := "[Descartar]"
				if action == "mouse-preserve" {
					label = "[Preservar]"
				}
				x, y = safetyModalLabelCell(t, m, label, true)
				drive(t, m, mousePress(m, x, y))
			} else {
				key := "enter"
				if action == "keyboard-preserve" {
					key = "u"
				}
				if action == "cancel" {
					key = "esc"
				}
				drive(t, m, press(m, key))
			}
			data, err := os.ReadFile(filepath.Join(dir, "new.go"))
			if strings.HasSuffix(action, "delete") {
				if !os.IsNotExist(err) {
					t.Fatal("confirmed deletion failed")
				}
			} else if err != nil || string(data) != "new code\n" {
				t.Fatal("preserve/cancel lost file")
			}
			raw, err := r.Exec(ctx, dir, "diff", "--cached", "--name-only")
			if err != nil {
				t.Fatal(err)
			}
			if action == "cancel" {
				if !strings.Contains(raw, "new.go") {
					t.Fatal("cancel changed stage")
				}
			} else if strings.Contains(raw, "new.go") {
				t.Fatal("file left staged")
			}
			if _, err := os.Stat(filepath.Join(dir, ".env")); err != nil {
				t.Fatal("ignored file affected")
			}
		})
	}
}

type cancellablePlanService struct {
	app.Service
	started chan context.Context
}

func (s *cancellablePlanService) PrepareDiscard(ctx context.Context, dir, path string) (*domain.DiscardPlan, error) {
	s.started <- ctx
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestDiscardReadCancelledAndLateReplyIgnored(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	s := &cancellablePlanService{Service: core, started: make(chan context.Context, 1)}
	m.core = s
	cmd := press(m, "x")
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var ctx context.Context
	select {
	case ctx = <-s.started:
	case <-time.After(time.Second):
		t.Fatal("read did not start")
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("missing read timeout")
	}
	press(m, "esc")
	select {
	case msg := <-done:
		m.Update(msg)
	case <-time.After(time.Second):
		t.Fatal("cancel did not stop read")
	}
	if ctx.Err() != context.Canceled || m.modal != "" || m.discardPlan != nil || m.busy != "" {
		t.Fatal("cancelled reply revived dialog")
	}
}

func TestBarsSeparatorsColorsAndConflicts(t *testing.T) {
	color256(t)
	m, _, _, _, _ := setupStudio(t)
	m.width = 180
	for _, ab := range [][2]int{{0, 0}, {2, 0}, {0, 3}, {2, 3}} {
		bar := m.contextBar("main", ab[0], ab[1])
		if strings.Count(ansi.Strip(bar), "|") != 3 {
			t.Fatal("context separators")
		}
		style := dimStyle
		if ab[0] > 0 {
			style = greenStyle
		}
		if ab[1] > 0 {
			style = yellowStyle
		}
		if !strings.Contains(bar, style.Render(fmt.Sprintf("↑%d ↓%d", ab[0], ab[1]))) {
			t.Fatal("sync color")
		}
	}
	m.studio.Changes = &domain.ChangeSet{Files: []domain.FileStatus{{Path: "conflict", Staged: "U", Unstaged: "U"}, {Path: "both", Staged: "M", Unstaged: "M"}, {Path: "new", Staged: "?", Unstaged: "?"}}}
	m.snapshotSummary = m.studio.Changes.Summary()
	bar := m.summaryBar()
	if strings.Count(ansi.Strip(bar), "|") != 5 || !strings.Contains(bar, redStyle.Bold(true).Render("!1 CONFLITOS")) {
		t.Fatal("critical summary missing separators or conflict alert")
	}
}
