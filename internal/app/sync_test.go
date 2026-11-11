package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

// seedFeature publishes main, then starts feat/x with one local commit.
func seedFeature(t *testing.T, path, content string) (*app.App, *git.NativeRunner, string) {
	t.Helper()
	a, r, dir, _ := seedPull(t)
	ctx := context.Background()
	runCmd(t, dir, "git", "switch", "-c", "feat/x")
	writeCommitFile(t, dir, path, content)
	if err := a.StageFiles(ctx, dir, []string{path}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "feat: local", false); err != nil {
		t.Fatal(err)
	}
	return a, r, dir
}

func TestSyncMergesRemoteBaseWithoutRewritingHistory(t *testing.T) {
	a, r, dir := seedFeature(t, "local.txt", "local\n")
	ctx := context.Background()
	localHead, _ := r.RevParseHead(ctx, dir)
	remoteOID := advanceRemote(t, a, r, dir, "remote.txt", "remote\n")
	target, err := a.GetSyncTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.Remote != "origin" || target.Base != "main" || target.Branch != "feat/x" || target.OnBase || target.Behind != 0 {
		t.Fatalf("target %+v (preview must not fetch)", target)
	}
	result, err := a.SyncFromBase(ctx, dir, *target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Incoming != 1 || result.Stopped || result.UpToDate {
		t.Fatalf("result %+v", result)
	}
	parents, _ := r.Exec(ctx, dir, "log", "-1", "--format=%P %s")
	if !strings.Contains(parents, localHead) || !strings.Contains(parents, remoteOID) || !strings.Contains(parents, "Merge remote-tracking branch 'origin/main' into feat/x") {
		t.Fatalf("merge commit %q", parents)
	}
	if out, _ := r.Exec(ctx, dir, "for-each-ref", "refs/vorq/"); strings.TrimSpace(out) != "" {
		t.Fatalf("temporary ref left: %q", out)
	}
	again, err := a.GetSyncTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result, err = a.SyncFromBase(ctx, dir, *again); err != nil || !result.UpToDate {
		t.Fatalf("second sync %+v %v", result, err)
	}
}

func TestSyncStopsOnConflictAndAborts(t *testing.T) {
	a, r, dir := seedFeature(t, "file.txt", "local side\n")
	ctx := context.Background()
	before, _ := r.RevParseHead(ctx, dir)
	advanceRemote(t, a, r, dir, "file.txt", "remote side\n")
	target, err := a.GetSyncTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.SyncFromBase(ctx, dir, *target)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stopped || len(result.Conflicts) != 1 || result.Conflicts[0] != "file.txt" {
		t.Fatalf("result %+v", result)
	}
	pending, err := a.GetSyncTarget(ctx, dir)
	if err != nil || !pending.MergeInProgress || len(pending.Conflicts) != 1 {
		t.Fatalf("pending %+v %v", pending, err)
	}
	if _, err := a.SyncFromBase(ctx, dir, *target); err == nil {
		t.Fatal("sync accepted while a merge is in progress")
	}
	if err := a.AbortMerge(ctx, dir); err != nil {
		t.Fatal(err)
	}
	after, _ := r.RevParseHead(ctx, dir)
	if after != before {
		t.Fatalf("HEAD %s, want %s", after, before)
	}
	if err := a.AbortMerge(ctx, dir); err == nil {
		t.Fatal("abort accepted without a merge in progress")
	}
}

func TestSyncBlocksDirtyTreeStaleReviewAndRoutesBase(t *testing.T) {
	a, r, dir := seedFeature(t, "local.txt", "local\n")
	ctx := context.Background()
	target, err := a.GetSyncTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "local.txt", "dirty\n")
	if _, err := a.GetSyncTarget(ctx, dir); err == nil || !strings.Contains(err.Error(), "commit or discard") {
		t.Fatalf("dirty tree: %v", err)
	}
	runCmd(t, dir, "git", "checkout", "--", "local.txt")
	writeCommitFile(t, dir, "other.txt", "x\n")
	if err := a.StageFiles(ctx, dir, []string{"other.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "feat: more", false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncFromBase(ctx, dir, *target); err == nil || !strings.Contains(err.Error(), "review the update again") {
		t.Fatalf("stale review: %v", err)
	}
	runCmd(t, dir, "git", "switch", "main")
	onBase, err := a.GetSyncTarget(ctx, dir)
	if err != nil || !onBase.OnBase {
		t.Fatalf("on base %+v %v", onBase, err)
	}
	if _, err := a.SyncFromBase(ctx, dir, *onBase); err == nil {
		t.Fatal("sync accepted on the base branch")
	}
	_ = r
}
