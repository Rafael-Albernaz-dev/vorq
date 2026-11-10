package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func spawnRepo(t *testing.T, branch string) (*app.App, string) {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-b", branch)
	runCmd(t, dir, "git", "config", "user.name", "Test User")
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "add", "README.md")
	runCmd(t, dir, "git", "commit", "-m", "init")
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	return app.NewApp(git.NewRepository(runner)), dir
}

// VORQ-012 (B5): without --base, spawn uses the repository's default branch.
func TestSpawnDefaultBaseFollowsRepository(t *testing.T) {
	ctx := context.Background()
	t.Run("master repository", func(t *testing.T) {
		a, dir := spawnRepo(t, "master")
		res, err := a.SpawnWorkspace(ctx, dir, app.SpawnOptions{BranchName: "feat/x", TargetDir: filepath.Join(t.TempDir(), "x")})
		if err != nil || res.BaseBranch != "master" {
			t.Fatalf("base=%v err=%v", res, err)
		}
	})
	t.Run("remote default branch", func(t *testing.T) {
		a, dir := spawnRepo(t, "main")
		runCmd(t, dir, "git", "branch", "develop")
		runCmd(t, dir, "git", "update-ref", "refs/remotes/origin/develop", "HEAD")
		runCmd(t, dir, "git", "remote", "add", "origin", t.TempDir())
		runCmd(t, dir, "git", "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
		runCmd(t, dir, "git", "switch", "-c", "other")
		res, err := a.SpawnWorkspace(ctx, dir, app.SpawnOptions{BranchName: "feat/y", TargetDir: filepath.Join(t.TempDir(), "y")})
		if err != nil || res.BaseBranch != "develop" {
			t.Fatalf("want develop, got %+v err=%v", res, err)
		}
	})
	t.Run("current HEAD fallback", func(t *testing.T) {
		a, dir := spawnRepo(t, "trunk")
		res, err := a.SpawnWorkspace(ctx, dir, app.SpawnOptions{BranchName: "feat/z", TargetDir: filepath.Join(t.TempDir(), "z")})
		if err != nil || res.BaseBranch != "trunk" {
			t.Fatalf("want trunk, got %+v err=%v", res, err)
		}
	})
}

// --copy is validated before anything is created.
func TestSpawnRejectsUnsafeOrMissingCopyFiles(t *testing.T) {
	ctx := context.Background()
	a, dir := spawnRepo(t, "main")
	outside := filepath.Join(filepath.Dir(dir), "outside-secret")
	if err := os.WriteFile(outside, []byte("s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.env")); err != nil {
		t.Fatal(err)
	}
	for name, copy := range map[string]string{"parent": "../outside-secret", "absolute": outside, "missing": "nope.env", "symlink": "link.env", "metadata": ".git/config"} {
		target := filepath.Join(t.TempDir(), name)
		_, err := a.SpawnWorkspace(ctx, dir, app.SpawnOptions{BranchName: "feat/" + name, TargetDir: target, CopyFiles: []string{copy}})
		if err == nil || !strings.Contains(err.Error(), "--copy") {
			t.Fatalf("%s: want --copy error, got %v", name, err)
		}
		if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
			t.Fatalf("%s: worktree created before validation", name)
		}
		out, _ := git.NewRepository(mustRunner(t)).Exec(ctx, dir, "branch", "--list", "feat/"+name)
		if strings.TrimSpace(out) != "" {
			t.Fatalf("%s: branch created before validation", name)
		}
	}
}

func mustRunner(t *testing.T) *git.NativeRunner {
	t.Helper()
	r, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
