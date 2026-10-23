package git_test

import (
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func TestParseWorktreeList(t *testing.T) {
	// Simulated NUL-delimited output
	raw := "worktree /home/user/project\x00" +
		"HEAD 0622cc19b1b8a4e8387a874b7a35263602dc7472\x00" +
		"branch refs/heads/main\x00\x00" +
		"worktree /home/user/project-worktrees/feat 123 (teste)\x00" +
		"HEAD a1b2c3d4e5f6\x00" +
		"branch refs/heads/feat/teste-com-espacos\x00\x00" +
		"worktree /home/user/project-worktrees/detached-one\x00" +
		"HEAD 998877665544\x00" +
		"detached\x00\x00" +
		"worktree /home/user/project-worktrees/locked-one\x00" +
		"HEAD 112233445566\x00" +
		"branch refs/heads/fix/locked\x00" +
		"locked on battery power\x00\x00"

	worktrees, err := git.ParseWorktreeList(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing worktrees: %v", err)
	}

	if len(worktrees) != 4 {
		t.Fatalf("expected 4 worktrees, got %d", len(worktrees))
	}

	// 1: Main worktree
	if !worktrees[0].IsMain {
		t.Errorf("expected worktree[0] to be main")
	}
	if worktrees[0].Branch != "main" {
		t.Errorf("expected branch 'main', got %q", worktrees[0].Branch)
	}
	if worktrees[0].Path != "/home/user/project" {
		t.Errorf("expected path '/home/user/project', got %q", worktrees[0].Path)
	}

	// 2: Path with spaces
	if worktrees[1].IsMain {
		t.Errorf("expected worktree[1] not to be main")
	}
	if worktrees[1].Branch != "feat/teste-com-espacos" {
		t.Errorf("expected branch 'feat/teste-com-espacos', got %q", worktrees[1].Branch)
	}
	if worktrees[1].Path != "/home/user/project-worktrees/feat 123 (teste)" {
		t.Errorf("expected path with spaces preserved, got %q", worktrees[1].Path)
	}

	// 3: Detached HEAD
	if !worktrees[2].IsDetached {
		t.Errorf("expected worktree[2] to be detached")
	}
	if worktrees[2].Branch != "(detached HEAD)" {
		t.Errorf("expected branch '(detached HEAD)', got %q", worktrees[2].Branch)
	}

	// 4: Locked with reason
	if !worktrees[3].IsLocked {
		t.Errorf("expected worktree[3] to be locked")
	}
	if worktrees[3].LockReason != "on battery power" {
		t.Errorf("expected lock reason 'on battery power', got %q", worktrees[3].LockReason)
	}
}

func TestParseWorktreeList_Empty(t *testing.T) {
	worktrees, err := git.ParseWorktreeList("")
	if err != nil {
		t.Fatalf("unexpected error on empty output: %v", err)
	}
	if len(worktrees) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(worktrees))
	}
}
