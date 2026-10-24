package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func BenchmarkHighlightGo256KiB(b *testing.B) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	source := strings.Repeat("// comment\nfunc example() string { return \"ação\" }\n", 5200)
	source = source[:256*1024]
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for b.Loop() {
		highlightCodeLines("main.go", source, "Tokyo Night")
	}
}

func BenchmarkTreeRebuild20k(b *testing.B) {
	m := NewModel(context.Background(), nil, "/workspace")
	files := make([]domain.FileStatus, 20000)
	for i := range files {
		files[i] = domain.FileStatus{Path: fmt.Sprintf("nested/f%05d.go", i), Unstaged: "M", Staged: " "}
	}
	m.studio = &domain.WorkspaceTree{Root: &domain.TreeNode{IsDir: true}, Changes: &domain.ChangeSet{Files: files}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.rebuildTree()
	}
}

func BenchmarkViewerCachedTabs256KiB(b *testing.B) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(old)
	source := strings.Repeat("package main\n// cached source\n", 10000)[:256*1024]
	v := viewerState{path: "f.go", mode: "arquivo", theme: "Nord", content: &domain.FileContent{Text: source}}
	if err := v.prepareModes(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		v.setMode("trabalho")
		v.setMode("arquivo")
	}
}
