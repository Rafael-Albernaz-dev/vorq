package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func sampleReview() *domain.DiffReview {
	return &domain.DiffReview{
		Summary: "Overall the change is risky: it adds a division helper without a divisor check and calls it with zero.\nVerify the intent before committing.",
		Findings: []domain.ReviewFinding{
			{FileID: "F0001", Severity: "info", Message: "Truncation semantics are undocumented.", Evidence: "+func div(a, b int) int { return a / b }"},
			{FileID: "F0001", Severity: "error", Message: "main always divides by zero and panics at startup.", Evidence: "+func main() { fmt.Println(div(1, 0)) }", Suggestion: "Validate b and return an error; handle it in main."},
			{FileID: "F0002", Severity: "warning", Message: "No tests cover the new helper \x1b[31minjected\x1b[0m.", Evidence: "diff --git a/main.go b/main.go\n(no *_test.go in the diff)", Suggestion: "Add table-driven tests for div."},
		},
		Dropped: 1,
		Snapshot: domain.ChangeSnapshot{Files: []domain.SnapshotFile{
			{ID: "F0001", Path: "cmd/app/main.go"}, {ID: "F0002", Path: "cmd/app/main_test.go"},
			{ID: "F0003", Path: "assets/logo.png", Excluded: "binary: handle manually"},
		}},
	}
}

func TestReviewLayoutSeparatesSectionsAndCards(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 60})
	m.diffReview, m.modal = sampleReview(), "diff-review"
	view := ansi.Strip(m.modalView())
	if os.Getenv("VORQ_DUMP_REVIEW") != "" {
		_ = os.WriteFile(os.Getenv("VORQ_DUMP_REVIEW"), []byte(view), 0600)
	}
	for _, want := range []string{"RESUMO", "1 erro(s)", "1 aviso(s)", "1 info", "1 descartado(s)", "1 arquivo(s) não analisado(s)", "Evidência", "Sugestão", "NÃO ANALISADOS", "cmd/app/main.go", "assets/logo.png"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q", want)
		}
	}
	// Most severe first, each finding numbered as its own card.
	e, w, i := strings.Index(view, "● ERRO  1/3"), strings.Index(view, "▲ AVISO  2/3"), strings.Index(view, "ℹ INFO  3/3")
	if e < 0 || w < 0 || i < 0 || !(e < w && w < i) {
		t.Fatalf("findings not ordered by severity:\n%s", view)
	}
	if strings.Contains(m.modalView(), "\x1b[31minjected") {
		t.Fatal("provider escape sequences must be neutralised")
	}
	assertFrame(t, m)
}
