package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
)

func TestInventoryIgnoredPagingAndAbsolutePaths(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", "cache/\n.env\n")
	writeCommitFile(t, dir, "tracked.txt", "tracked\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "test: inventory baseline"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 205; i++ {
		writeCommitFile(t, dir, fmt.Sprintf("cache/arquivo %03d ação.txt", i), "ignored\n")
	}
	writeCommitFile(t, dir, ".env", "private\n")
	writeCommitFile(t, dir, "novo.txt", "new\n")
	before, _ := a.GetCommitChanges(ctx, dir)
	tree, err := a.GetWorkspaceTree(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cache := findTreeNode(tree.Root, "cache")
	if cache == nil || !cache.Lazy || len(cache.Children) != 0 || cache.Tracking != "ignored" {
		t.Fatalf("not lazy: %+v", cache)
	}
	if findTreeNode(tree.Root, "novo.txt").Tracking != "untracked" || findTreeNode(tree.Root, "tracked.txt").Tracking != "tracked" {
		t.Fatal("tracking conflated")
	}
	first, err := a.ReadInventoryDirectory(ctx, dir, "cache", 0)
	if err != nil || len(first) != app.InventoryPageSize+1 || !first[len(first)-1].More {
		t.Fatalf("first page: %d %v", len(first), err)
	}
	second, err := a.ReadInventoryDirectory(ctx, dir, "cache", first[len(first)-1].MoreOffset)
	if err != nil || len(second) != 5 {
		t.Fatalf("second page: %d %v", len(second), err)
	}
	for _, n := range second {
		if n.Tracking != "ignored" {
			t.Fatal("ignored descendant classification")
		}
	}
	path := "cache/arquivo 204 ação.txt"
	absolute, err := a.AbsoluteFilePath(ctx, dir, path)
	if err != nil || absolute != filepath.Join(dir, path) {
		t.Fatalf("absolute path: %q %v", absolute, err)
	}
	if err = os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(dir, "reference")); err != nil {
		t.Fatal(err)
	}
	absolute, err = a.AbsoluteFilePath(ctx, dir, "reference")
	if err != nil || absolute != filepath.Join(dir, "reference") {
		t.Fatal("symlink path resolved externally")
	}
	if _, err = a.ReadInventoryDirectory(ctx, dir, "reference", 0); err == nil {
		t.Fatal("followed directory symlink")
	}
	for _, p := range []string{".git/config", "../outside", dir} {
		if _, err = a.AbsoluteFilePath(ctx, dir, p); err == nil {
			t.Fatalf("unsafe path %q", p)
		}
	}
	after, _ := a.GetCommitChanges(ctx, dir)
	if before.StagedDiff != after.StagedDiff || after.StagedCount() != 0 || after.UntrackedCount() != 2 {
		t.Fatal("inventory mutated changes/stage")
	}
}
