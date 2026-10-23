package domain_test

import (
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func TestFileStatus_ConflictsAreNotStaged(t *testing.T) {
	for _, status := range []domain.FileStatus{{Staged: "U", Unstaged: "U"}, {Staged: "A", Unstaged: "A"}, {Staged: "D", Unstaged: "D"}} {
		if !status.IsConflicted() || status.IsStaged() {
			t.Fatalf("conflict misclassified: %+v", status)
		}
	}
}

func TestChangeSet_IsClean(t *testing.T) {
	csEmpty := domain.ChangeSet{
		WorkspacePath: "/path/to/ws",
		Files:         []domain.FileStatus{},
	}
	if !csEmpty.IsClean() {
		t.Errorf("expected empty changeset to be clean")
	}

	csDirty := domain.ChangeSet{
		WorkspacePath: "/path/to/ws",
		Files: []domain.FileStatus{
			{Path: "main.go", Staged: "M", Unstaged: ""},
			{Path: "app.go", Staged: "", Unstaged: "M"},
			{Path: "new.go", Staged: "?", Unstaged: "?"},
		},
	}
	if csDirty.IsClean() {
		t.Errorf("expected changeset with files not to be clean")
	}
	if got := csDirty.StagedCount(); got != 1 {
		t.Errorf("expected StagedCount 1, got %d", got)
	}
	if got := csDirty.UnstagedCount(); got != 1 {
		t.Errorf("expected UnstagedCount 1, got %d", got)
	}
	if got := csDirty.UntrackedCount(); got != 1 {
		t.Errorf("expected UntrackedCount 1, got %d", got)
	}
}

func TestCommitProposal_Message(t *testing.T) {
	propSubjectOnly := domain.CommitProposal{
		Subject:     "feat: add initial foundation",
		Engine:      domain.EngineDeterministicRules,
		GeneratedAt: time.Now(),
	}
	if got := propSubjectOnly.Message(); got != "feat: add initial foundation" {
		t.Errorf("expected %q, got %q", "feat: add initial foundation", got)
	}

	propWithBody := domain.CommitProposal{
		Subject: "fix: prevent data loss",
		Body:    "Inspect ignored files before teardown.",
		Engine:  domain.EngineLocalModel,
	}
	expected := "fix: prevent data loss\n\nInspect ignored files before teardown."
	if got := propWithBody.Message(); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestChangeSet_SummaryCountsUniquePaths(t *testing.T) {
	cs := &domain.ChangeSet{Files: []domain.FileStatus{
		{Path: "partial.go", Staged: "M", Unstaged: "M"},
		{Path: "partial.go", Staged: "M", Unstaged: " "},
		{Path: "new.txt", Staged: "?", Unstaged: "?"},
		{Path: "conflict.go", Staged: "U", Unstaged: "U"},
		{Path: "clean.go", Staged: " ", Unstaged: " "},
	}}
	got := cs.Summary()
	if got.Total != 3 || got.Staged != 1 || got.Unstaged != 1 || got.Untracked != 1 || got.Conflicts != 1 || got.Both != 1 {
		t.Fatalf("summary %+v", got)
	}
	root := &domain.TreeNode{IsDir: true, Children: []*domain.TreeNode{{Path: "clean.go"}, {Name: "src", IsDir: true, Children: []*domain.TreeNode{{Path: "src/file.go"}}}}}
	if root.FileCount() != 2 {
		t.Fatal("directories counted as files")
	}
}
