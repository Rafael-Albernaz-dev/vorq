package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

// VORQ-011: a branch whose upstream was pruned ("[gone]", e.g. after a merged
// PR and fetch --prune) is a normal state, not a failure of every read.
func TestAheadBehindToleratesGoneUpstream(t *testing.T) {
	ctx := context.Background()
	runner, dir := setupRunnerRepo(t)
	remote := t.TempDir()
	if _, err := runner.Exec(ctx, remote, "init", "--bare", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	writeRunnerFile(t, dir, "a.txt", "a\n")
	for _, args := range [][]string{{"add", "a.txt"}, {"commit", "-m", "init"}, {"remote", "add", "origin", remote}, {"switch", "-c", "feat"}, {"push", "-u", "origin", "feat"}, {"push", "origin", "--delete", "feat"}, {"fetch", "--prune"}} {
		if _, err := runner.Exec(ctx, dir, args...); err != nil {
			t.Fatal(args, err)
		}
	}
	ahead, behind, err := runner.AheadBehind(ctx, dir)
	if err != nil || ahead != 0 || behind != 0 {
		t.Fatalf("gone upstream: ahead=%d behind=%d err=%v", ahead, behind, err)
	}
}

func TestIgnoredAmongUsesGitExcludesInOneCall(t *testing.T) {
	ctx := context.Background()
	runner, dir := setupRunnerRepo(t)
	writeRunnerFile(t, dir, ".gitignore", "node_modules/\n*.log\n")
	for _, p := range []string{"node_modules/pkg/lib", "src"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeRunnerFile(t, dir, "node_modules/pkg/lib/a.js", "x\n")
	writeRunnerFile(t, dir, "src/ok.txt", "x\n")
	writeRunnerFile(t, dir, "src/b.log", "x\n")
	repo := git.NewRepository(runner)
	got, err := repo.IgnoredAmong(ctx, dir, []string{"node_modules/pkg", "src", "src/ok.txt", "src/b.log", ".gitignore"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"node_modules/pkg": true, "src/b.log": true}
	if len(got) != len(want) || !got["node_modules/pkg"] || !got["src/b.log"] {
		t.Fatalf("ignored = %v, want %v", got, want)
	}
	none, err := repo.IgnoredAmong(ctx, dir, []string{"src/ok.txt"})
	if err != nil || len(none) != 0 {
		t.Fatalf("no ignored path must not fail: %v %v", none, err)
	}
}

// Reads issued by the Studio must never rewrite .git/index (which takes
// index.lock and races with git running in another terminal).
func TestStudioReadsDoNotRewriteIndex(t *testing.T) {
	ctx := context.Background()
	runner, dir := setupRunnerRepo(t)
	writeRunnerFile(t, dir, ".gitignore", "*.log\n")
	writeRunnerFile(t, dir, "a.txt", "a\n")
	writeRunnerFile(t, dir, "x.log", "x\n")
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "init"}} {
		if _, err := runner.Exec(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Make the cached stat data stale so a refreshing status would rewrite it.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), future, future); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, ".git", "index")
	before, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	repo := git.NewRepository(runner)
	if _, err := repo.IgnoredFiles(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadChanges(ctx, dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Status(ctx, dir); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatal("a read-only Studio query rewrote .git/index")
	}
}
