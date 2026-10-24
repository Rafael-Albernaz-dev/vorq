package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

func TestVisualChangesAreFlatAndRepositoryRetainsHierarchy(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	writeStudio(t, dir, "nested/new.go", "package nested\n")
	drive(t, m, m.refresh())
	for _, row := range m.rows {
		if row.node != nil && row.node.IsDir {
			t.Fatalf("Changes wastes a row on directory %s", row.node.Path)
		}
	}
	selectPath(t, m, "nested/new.go")
	text := ansi.Strip(m.treeView(60))
	line := ""
	for _, candidate := range strings.Split(text, "\n") {
		if strings.Contains(candidate, "new.go") {
			line = candidate
		}
	}
	if !strings.HasPrefix(line, "›? [ ] 🔷") {
		t.Fatalf("status, stage, type and name must remain separate: %q", line)
	}
	m.switchTree("repositorio")
	folder := false
	for _, row := range m.rows {
		if row.node != nil && row.node.IsDir && row.node.Path == "nested" {
			folder = true
		}
	}
	if !folder {
		t.Fatal("Repository lost its complete hierarchy")
	}
}

func TestVisualDiffHasSingleGutterAndPreservesGitLines(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	v := &m.viewer
	text := ansi.Strip(strings.Join(v.lines, "\n"))
	for _, metadata := range []string{"diff --git", "index ", "--- a/", "+++ b/"} {
		if strings.Contains(text, metadata) {
			t.Fatalf("Git metadata leaked into code viewport: %q", text)
		}
	}
	if !strings.Contains(text, "3 + // changed") || v.added != 2 || v.deleted != 0 {
		t.Fatalf("native patch lost lines/count: added %d, removed %d: %s", v.added, v.deleted, text)
	}
	found := false
	for i, line := range v.lines {
		if strings.Contains(line, "// changed") {
			found = true
			if v.lineBackgrounds[i] != ColorAddedBg {
				t.Fatal("added line lost its distinct full-row surface")
			}
		}
	}
	if !found {
		t.Fatal("changed line missing")
	}
}

func TestVisualBinaryDoesNotFabricateTextOrLineCounts(t *testing.T) {
	v := viewerState{mode: "stage", diff: &domain.FileDiff{Staged: "diff --git a/image.png b/image.png\nBinary files a/image.png and b/image.png differ\n"}}
	v.rebuild()
	if !v.binary || v.added != 0 || v.deleted != 0 || !strings.Contains(ansi.Strip(strings.Join(v.lines, "\n")), "sem prévia textual") {
		t.Fatal("binary preview misrepresented as textual patch")
	}
}

func TestVisualTabsKeepLabelsAndCellBudget(t *testing.T) {
	for _, width := range []int{12, 34, 61, 140} {
		for active := 0; active < 3; active++ {
			rendered := strings.Split(renderTabs([]string{"Trabalho", "Stage", "Arquivo"}, active, width), "\n")
			if len(rendered) != 3 {
				t.Fatal("tab grid must have three rows")
			}
			for _, row := range rendered {
				if ansi.StringWidth(row) != width {
					t.Fatalf("tab width %d exceeds cell budget %d", ansi.StringWidth(row), width)
				}
			}
			if width >= 34 && (!strings.Contains(ansi.Strip(rendered[1]), "Trabalho") || !strings.Contains(ansi.Strip(rendered[1]), "Arquivo")) {
				t.Fatal("visible navigation labels lost")
			}
		}
	}
}

func TestVisualNativeStatusAndIntentionalFileTypes(t *testing.T) {
	for code, want := range map[string]string{"T": "T", "C": "C", "R": "R", "M": "M", "A": "A", "D": "D"} {
		if got := ansi.Strip(statusBadge(domain.FileStatus{Staged: code})); got != want {
			t.Fatalf("native status %s misrepresented as %s", code, got)
		}
	}
	for name, want := range map[string]string{"go.mod": "[MOD]", "go.sum": "[MOD]", ".gitignore": "[GIT]", "check.yml": "[YML]", "script.sh": "[SH]", "logo.png": "[IMG]", "LICENSE": "[TXT]"} {
		if got := fileIcon(&domain.TreeNode{Name: name}, false); got != want {
			t.Fatalf("special file %s has incorrect type %s", name, got)
		}
	}
}

func TestVisualUnknownInventoryNeverClaimsClean(t *testing.T) {
	m := NewModel(context.Background(), nil, "/workspace")
	for _, tab := range []string{"alteracoes", "repositorio"} {
		m.activeTab = tab
		m.loading = true
		text := ansi.Strip(m.treeView(80))
		if !strings.Contains(text, "Lendo inventário") || strings.Contains(text, "limpo") || strings.Contains(text, "sem arquivos") {
			t.Fatalf("unknown inventory presented as empty: %q", text)
		}
		m.loading = false
		m.stale = true
		text = ansi.Strip(m.treeView(80))
		if !strings.Contains(text, "Inventário indisponível") || strings.Contains(text, "limpo") {
			t.Fatalf("failed inventory presented as clean: %q", text)
		}
	}
}

func TestVisualDirectorySummaryUsesUniqueDescendants(t *testing.T) {
	node := &domain.TreeNode{Path: "internal", IsDir: true, Children: []*domain.TreeNode{
		{Path: "internal/clean.go"},
		{Path: "internal/both.go", Status: domain.FileStatus{Staged: "M", Unstaged: "M"}},
		{Path: "internal/new.go", Status: domain.FileStatus{Staged: "?", Unstaged: "?"}},
	}}
	v := viewerState{directory: node}
	v.rebuild()
	text := ansi.Strip(strings.Join(v.lines, "\n"))
	for _, want := range []string{"Caminhos descendentes  3", "Sem alterações         1", "Alterados              2", "No stage               1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("directory summary lost native union count %q: %s", want, text)
		}
	}
}

func TestVisualReadFailureIsNotAnEmptyFile(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	drive(t, m, press(m, "e"))
	// The repository snapshot is valid; an external deletion happens between
	// discovery and opening the clean file, producing a real filesystem error.
	if err := os.Remove(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}
	selectPath(t, m, "config.json")
	text := ansi.Strip(strings.Join(m.viewer.lines, "\n"))
	if m.viewer.content == nil || m.viewer.content.Notice == "" || strings.Contains(text, "Arquivo vazio") || !strings.Contains(text, "config.json") {
		t.Fatalf("real failed read disguised as empty file: %q", text)
	}
}
