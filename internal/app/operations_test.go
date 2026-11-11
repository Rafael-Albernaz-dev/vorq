package app_test

import (
	"context"
	"strings"
	"testing"
)

func TestCherryPickAppliesAndStopsOnConflict(t *testing.T) {
	a, r, dir := seedFeature(t, "local.txt", "local\n")
	ctx := context.Background()
	runCmd(t, dir, "git", "switch", "-q", "-c", "other", "main")
	writeCommitFile(t, dir, "pick.txt", "pick\n")
	runCmd(t, dir, "git", "add", "pick.txt")
	runCmd(t, dir, "git", "commit", "-q", "-m", "feat: pick me")
	pick, _ := r.RevParseHead(ctx, dir)
	runCmd(t, dir, "git", "switch", "-q", "feat/x")
	plan, err := a.PrepareCherryPick(ctx, dir, pick)
	if err != nil || plan.Subject != "feat: pick me" || plan.Branch != "feat/x" {
		t.Fatalf("plan %+v %v", plan, err)
	}
	result, err := a.CherryPick(ctx, dir, *plan)
	if err != nil || result.Stopped {
		t.Fatalf("%+v %v", result, err)
	}
	msg, _ := r.Exec(ctx, dir, "log", "-1", "--format=%B")
	if !strings.Contains(msg, "cherry picked from commit "+pick) {
		t.Fatalf("message %q", msg)
	}
	if _, err := a.PrepareCherryPick(ctx, dir, pick); err == nil || !strings.Contains(err.Error(), "already in") {
		t.Fatalf("already applied: %v", err)
	}
	// Conflict: the same file changed differently on both sides.
	runCmd(t, dir, "git", "switch", "-q", "other")
	writeCommitFile(t, dir, "local.txt", "theirs\n")
	runCmd(t, dir, "git", "add", "local.txt")
	runCmd(t, dir, "git", "commit", "-q", "-m", "feat: conflicting")
	conflicting, _ := r.RevParseHead(ctx, dir)
	runCmd(t, dir, "git", "switch", "-q", "feat/x")
	before, _ := r.RevParseHead(ctx, dir)
	plan, err = a.PrepareCherryPick(ctx, dir, conflicting)
	if err != nil {
		t.Fatal(err)
	}
	if result, err = a.CherryPick(ctx, dir, *plan); err != nil || !result.Stopped || len(result.Conflicts) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	state, _ := a.GetOperationState(ctx, dir)
	if state.Kind != "cherry-pick" {
		t.Fatalf("state %+v", state)
	}
	if _, err := a.ContinueOperation(ctx, dir); err == nil || !strings.Contains(err.Error(), "still in conflict") {
		t.Fatalf("continue with conflicts: %v", err)
	}
	// Resolve, stage, continue without an editor.
	writeCommitFile(t, dir, "local.txt", "resolved\n")
	runCmd(t, dir, "git", "add", "local.txt")
	if result, err = a.ContinueOperation(ctx, dir); err != nil || result.Stopped {
		t.Fatalf("continue %+v %v", result, err)
	}
	if after, _ := r.RevParseHead(ctx, dir); after == before {
		t.Fatal("continue did not commit")
	}
	if _, err := a.AbortOperation(ctx, dir); err == nil {
		t.Fatal("abort accepted with nothing in progress")
	}
}

func TestCherryPickRefusesMergeCommitsAndDirtyTree(t *testing.T) {
	a, _, dir := seedFeature(t, "local.txt", "local\n")
	ctx := context.Background()
	runCmd(t, dir, "git", "switch", "-q", "main")
	runCmd(t, dir, "git", "merge", "-q", "--no-ff", "-m", "merge feat", "feat/x")
	runCmd(t, dir, "git", "switch", "-q", "-c", "side", "main~1")
	if _, err := a.PrepareCherryPick(ctx, dir, "main"); err == nil || !strings.Contains(err.Error(), "merge") {
		t.Fatalf("merge commit: %v", err)
	}
	writeCommitFile(t, dir, "dirty.txt", "x\n")
	if _, err := a.PrepareCherryPick(ctx, dir, "feat/x"); err == nil || !strings.Contains(err.Error(), "commit or discard") {
		t.Fatalf("dirty: %v", err)
	}
}

func TestRebaseOnlyUnpublishedAndAbort(t *testing.T) {
	a, r, dir := seedFeature(t, "local.txt", "local\n")
	ctx := context.Background()
	advanceRemote(t, a, r, dir, "remote.txt", "remote\n")
	plan, err := a.PrepareRebase(ctx, dir)
	if err != nil || plan.Base != "main" || plan.Commits != 1 {
		t.Fatalf("plan %+v %v", plan, err)
	}
	result, err := a.Rebase(ctx, dir, *plan)
	if err != nil || result.Stopped {
		t.Fatalf("%+v %v", result, err)
	}
	parents, _ := r.Exec(ctx, dir, "log", "-1", "--format=%P")
	tracking, _ := r.Exec(ctx, dir, "rev-parse", "origin/main")
	if strings.TrimSpace(parents) != strings.TrimSpace(tracking) {
		t.Fatalf("not rebased onto origin/main: parent %q base %q", parents, tracking)
	}
	// Published commits are never rebased (no force push).
	runCmd(t, dir, "git", "push", "-q", "-u", "origin", "feat/x")
	if _, err := a.PrepareRebase(ctx, dir); err == nil || !strings.Contains(err.Error(), "already published") {
		t.Fatalf("published: %v", err)
	}
}

func TestRebaseConflictAbortRestores(t *testing.T) {
	a, r, dir := seedFeature(t, "file.txt", "local side\n")
	ctx := context.Background()
	advanceRemote(t, a, r, dir, "file.txt", "remote side\n")
	before, _ := r.RevParseHead(ctx, dir)
	plan, err := a.PrepareRebase(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Rebase(ctx, dir, *plan)
	if err != nil || !result.Stopped || len(result.Conflicts) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := a.PrepareRebase(ctx, dir); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("second rebase: %v", err)
	}
	if kind, err := a.AbortOperation(ctx, dir); err != nil || kind != "rebase" {
		t.Fatalf("abort %q %v", kind, err)
	}
	if after, _ := r.RevParseHead(ctx, dir); after != before {
		t.Fatalf("HEAD %s want %s", after, before)
	}
}
