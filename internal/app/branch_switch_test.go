package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func TestSwitchBranchLocalListAndCleanSwitch(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", ".env\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "chore: ignore env")
	if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feat/ação", Stay: true}); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, ".env", "private\n")
	branches, e := a.ListLocalBranches(ctx, dir)
	if e != nil || len(branches) != 2 {
		t.Fatalf("branches %+v %v", branches, e)
	}
	current := false
	for _, b := range branches {
		if b.Name == "main" {
			current = b.Current && b.WorktreePath != ""
		}
	}
	if !current {
		t.Fatal("current/worktree metadata absent")
	}
	plan, e := a.PrepareBranchSwitch(ctx, dir, "feat/ação")
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SwitchBranch(ctx, dir, *plan); e != nil {
		t.Fatal(e)
	}
	branch, _ := r.CurrentBranch(ctx, dir)
	raw, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if branch != "feat/ação" || string(raw) != "private\n" {
		t.Fatal("switch failed or ignored bytes lost")
	}
	if _, e = a.PrepareBranchSwitch(ctx, dir, "--detach"); e == nil {
		t.Fatal("arbitrary revision accepted")
	}
}
func TestSwitchBranchRejectsDirtyState(t *testing.T) {
	for _, kind := range []string{"staged", "unstaged", "untracked"} {
		t.Run(kind, func(t *testing.T) {
			a, r, dir := seedWorkflow(t)
			ctx := context.Background()
			_ = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feature", Stay: true})
			name := "file.txt"
			if kind == "untracked" {
				name = "new.txt"
			}
			writeCommitFile(t, dir, name, "preserve\n")
			if kind == "staged" {
				_ = r.Add(ctx, dir, name)
			}
			before, _ := r.Status(ctx, dir)
			if _, e := a.PrepareBranchSwitch(ctx, dir, "feature"); e == nil {
				t.Fatal("dirty switch allowed")
			}
			after, _ := r.Status(ctx, dir)
			branch, _ := r.CurrentBranch(ctx, dir)
			if branch != "main" || before != after {
				t.Fatal("blocked switch changed state")
			}
		})
	}
}
func TestSwitchBranchOccupiedWorktreeAndStalePlan(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	held := filepath.Join(t.TempDir(), "workspace with\nnewline")
	if _, e := r.Exec(ctx, dir, "worktree", "add", "-b", "held", held, "main"); e != nil {
		t.Fatal(e)
	}
	branches, e := a.ListLocalBranches(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, b := range branches {
		if b.Name == "held" {
			found = b.WorktreePath == held
		}
	}
	if !found {
		t.Fatal("worktree path serialization corrupted")
	}
	if _, e = a.PrepareBranchSwitch(ctx, dir, "held"); e == nil || !strings.Contains(e.Error(), "Worktrees") {
		t.Fatal("occupied branch not explained")
	}
	_ = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feature", Stay: true})
	plan, e := a.PrepareBranchSwitch(ctx, dir, "feature")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Exec(ctx, dir, "commit", "--allow-empty", "-m", "chore: concurrent head"); e != nil {
		t.Fatal(e)
	}
	if e = a.SwitchBranch(ctx, dir, *plan); e == nil {
		t.Fatal("stale source accepted")
	}
	head, _ := r.RevParseHead(ctx, dir)
	if _, e = r.Exec(ctx, dir, "update-ref", "refs/heads/main", plan.FromHash, head); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Exec(ctx, dir, "update-ref", "refs/heads/feature", head, plan.TargetHash); e != nil {
		t.Fatal(e)
	}
	if e = a.SwitchBranch(ctx, dir, *plan); e == nil {
		t.Fatal("stale destination accepted")
	}
	branch, _ := r.CurrentBranch(ctx, dir)
	if branch != "main" {
		t.Fatal("stale plan changed branch")
	}
}
func TestSwitchBranchProtectsIgnoredCollision(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", ".env\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "chore: ignore env")
	_ = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feature"})
	writeCommitFile(t, dir, ".env", "remote tracked\n")
	if _, e := r.Exec(ctx, dir, "add", "-f", "--", ".env"); e != nil {
		t.Fatal(e)
	}
	_, _ = r.Commit(ctx, dir, "test: tracked env")
	if _, e := r.Exec(ctx, dir, "switch", "main"); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, ".env", "private local\n")
	plan, e := a.PrepareBranchSwitch(ctx, dir, "feature")
	if e != nil {
		t.Fatal(e)
	}
	if e = a.SwitchBranch(ctx, dir, *plan); e == nil {
		t.Fatal("ignored collision accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".env"))
	branch, _ := r.CurrentBranch(ctx, dir)
	if string(raw) != "private local\n" || branch != "main" {
		t.Fatal("ignored file or branch changed")
	}
}
