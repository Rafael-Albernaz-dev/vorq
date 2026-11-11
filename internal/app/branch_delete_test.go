package app_test

import (
	"context"
	"strings"
	"testing"
)

func TestDeleteMergedBranchLocalAndRemote(t *testing.T) {
	a, r, dir, remote := seedPull(t)
	ctx := context.Background()
	runCmd(t, dir, "git", "branch", "feat/done")
	runCmd(t, dir, "git", "push", "-q", "-u", "origin", "feat/done")
	plan, err := a.PrepareBranchDelete(ctx, dir, "feat/done")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Merged || plan.Remote != "origin" || plan.RemoteHash == "" || plan.NeedsConfirm() || plan.Base != "main" {
		t.Fatalf("plan %+v", plan)
	}
	result, err := a.DeleteBranch(ctx, dir, *plan, false)
	if err != nil || !result.RemoteDeleted {
		t.Fatalf("%+v %v", result, err)
	}
	if out, _ := r.Exec(ctx, dir, "branch", "--list", "feat/done"); strings.TrimSpace(out) != "" {
		t.Fatal("local branch kept")
	}
	if out, _ := r.Exec(ctx, remote, "branch", "--list", "feat/done"); strings.TrimSpace(out) != "" {
		t.Fatal("remote branch kept")
	}
}

func TestDeleteUnmergedNeedsConfirmAndRefusals(t *testing.T) {
	a, r, dir := seedFeature(t, "local.txt", "local\n")
	ctx := context.Background()
	if _, err := a.PrepareBranchDelete(ctx, dir, "feat/x"); err == nil || !strings.Contains(err.Error(), "current branch") {
		t.Fatalf("current: %v", err)
	}
	runCmd(t, dir, "git", "switch", "-q", "-c", "tmp", "main")
	if _, err := a.PrepareBranchDelete(ctx, dir, "main"); err == nil || !strings.Contains(err.Error(), "default branch") {
		t.Fatalf("base: %v", err)
	}
	plan, err := a.PrepareBranchDelete(ctx, dir, "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Merged || plan.Unpublished != 1 || plan.RemoteHash != "" || !plan.NeedsConfirm() {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := a.DeleteBranch(ctx, dir, *plan, false); err == nil || !strings.Contains(err.Error(), "CONFIRMAR") {
		t.Fatalf("unconfirmed: %v", err)
	}
	stale := *plan
	stale.Hash = "0000"
	if _, err := a.DeleteBranch(ctx, dir, stale, true); err == nil {
		t.Fatal("stale plan accepted")
	}
	if _, err := a.DeleteBranch(ctx, dir, *plan, true); err != nil {
		t.Fatal(err)
	}
	if out, _ := r.Exec(ctx, dir, "branch", "--list", "feat/x"); strings.TrimSpace(out) != "" {
		t.Fatal("branch kept")
	}
}

func TestDeleteRemoteLeaseProtectsNewRemoteCommits(t *testing.T) {
	a, r, dir, remote := seedPull(t)
	ctx := context.Background()
	runCmd(t, dir, "git", "branch", "feat/shared")
	runCmd(t, dir, "git", "push", "-q", "-u", "origin", "feat/shared")
	plan, err := a.PrepareBranchDelete(ctx, dir, "feat/shared")
	if err != nil {
		t.Fatal(err)
	}
	// Someone else pushes to the remote branch after the review.
	other := t.TempDir()
	runCmd(t, other, "git", "clone", "-q", remote, "c")
	clone := other + "/c"
	runCmd(t, clone, "git", "switch", "-q", "feat/shared")
	runCmd(t, clone, "git", "-c", "user.name=O", "-c", "user.email=o@x.test", "commit", "-q", "--allow-empty", "-m", "theirs")
	runCmd(t, clone, "git", "push", "-q")
	if _, err := a.DeleteBranch(ctx, dir, *plan, false); err == nil || !strings.Contains(err.Error(), "not deleted") {
		t.Fatalf("lease: %v", err)
	}
	if out, _ := r.Exec(ctx, remote, "branch", "--list", "feat/shared"); strings.TrimSpace(out) == "" {
		t.Fatal("remote branch with new commits was deleted")
	}
	if out, _ := r.Exec(ctx, dir, "branch", "--list", "feat/shared"); strings.TrimSpace(out) == "" {
		t.Fatal("local branch deleted after remote failure")
	}
}

func TestDeleteBranchAlreadyGoneOnRemote(t *testing.T) {
	a, r, dir, remote := seedPull(t)
	ctx := context.Background()
	runCmd(t, dir, "git", "branch", "feat/auto")
	runCmd(t, dir, "git", "push", "-q", "-u", "origin", "feat/auto")
	runCmd(t, remote, "git", "branch", "-D", "feat/auto") // GitHub auto-delete
	plan, err := a.PrepareBranchDelete(ctx, dir, "feat/auto")
	if err != nil || plan.RemoteHash == "" {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if _, err := a.DeleteBranch(ctx, dir, *plan, false); err != nil {
		t.Fatal(err)
	}
	if out, _ := r.Exec(ctx, dir, "for-each-ref", "refs/remotes/origin/feat/auto", "refs/heads/feat/auto"); strings.TrimSpace(out) != "" {
		t.Fatalf("refs kept: %q", out)
	}
}
