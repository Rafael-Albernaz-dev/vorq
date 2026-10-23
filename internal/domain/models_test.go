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
