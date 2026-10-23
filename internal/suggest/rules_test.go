package suggest_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/suggest"
)

func TestInferScope(t *testing.T) {
	for _, tt := range []struct {
		name  string
		paths []string
		want  string
	}{
		{"ui", []string{"internal/ui/root.go"}, "ui"},
		{"git", []string{"internal/git/runner.go", "internal/git/runner_test.go"}, "git"},
		{"docs", []string{"docs/architecture.md"}, "docs"},
		{"root", []string{"README.md", "go.mod"}, ""},
		{"spread", []string{"internal/git/runner.go", "internal/ui/root.go"}, ""},
		{"majority", []string{"internal/git/a.go", "internal/git/b.go", "README.md"}, "git"},
		{"root tie", []string{"README.md", "internal/git/a.go"}, ""},
		{"unsafe delimiter", []string{"bad): scope/a"}, ""},
		{"empty", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var files []domain.FileStatus
			for _, path := range tt.paths {
				files = append(files, domain.FileStatus{Path: path, Staged: "M"})
			}
			files = append(files, domain.FileStatus{Path: "unstaged/secret", Staged: " ", Unstaged: "M"})
			if got := suggest.InferScope(files); got != tt.want {
				t.Fatalf("scope = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateSubject(t *testing.T) {
	for _, tt := range []struct{ status, want string }{
		{"A", "add runner.go"}, {"M", "update runner.go"}, {"D", "remove runner.go"},
		{"R", "rename runner.go"}, {"T", "update type of runner.go"},
	} {
		t.Run(tt.status, func(t *testing.T) {
			got := suggest.GenerateSubject("feat", []domain.FileStatus{{Path: "internal/git/runner.go", Staged: tt.status}})
			if got != tt.want {
				t.Fatalf("subject = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProposeMessage_DeterministicAndStagedOnly(t *testing.T) {
	files := []domain.FileStatus{
		{Path: "internal/git/z.go", Staged: "M", Unstaged: "M"},
		{Path: "private/secret", Staged: "?", Unstaged: "?"},
		{Path: "internal/git/a.go", Staged: "A"},
	}
	before := append([]domain.FileStatus(nil), files...)
	for _, commitType := range []string{"feat", "fix", "chore", "docs", "refactor", "test", "style", "perf"} {
		proposal := suggest.ProposeMessage(commitType, files)
		want := commitType + "(git): add a.go; update z.go"
		if proposal.Message() != want || proposal.Engine != domain.EngineDeterministicRules || proposal.GeneratedAt.IsZero() {
			t.Fatalf("unexpected proposal: %+v, want %q", proposal, want)
		}
		if !suggest.ValidType(commitType) {
			t.Fatalf("invalid supported type %q", commitType)
		}
	}
	if !reflect.DeepEqual(files, before) {
		t.Fatal("rules mutated input")
	}
	if suggest.ValidType("other") {
		t.Fatal("accepted unsupported type")
	}
}

func TestGenerateSubject_BoundedAndSingleLine(t *testing.T) {
	var files []domain.FileStatus
	for _, path := range []string{"a", "b", "c"} {
		files = append(files, domain.FileStatus{Path: path, Staged: "A"})
	}
	if got := suggest.GenerateSubject("feat", files); got != "add 3 files" {
		t.Fatalf("got %q", got)
	}
	got := suggest.GenerateSubject("fix", []domain.FileStatus{{Path: "line\nfeed\x1b.go", Staged: "M"}})
	if strings.ContainsAny(got, "\n\x1b") {
		t.Fatalf("unsafe subject %q", got)
	}
}
