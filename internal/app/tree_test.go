package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func findTreeNode(root *domain.TreeNode, path string) *domain.TreeNode {
	if root.Path == path {
		return root
	}
	for _, child := range root.Children {
		if n := findTreeNode(child, path); n != nil {
			return n
		}
	}
	return nil
}

func TestApp_WorkspaceTree_StatusAndIgnores(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", "bin/\nnode_modules/\n.env\n")
	writeCommitFile(t, dir, "src/ação.go", "original\n")
	writeCommitFile(t, dir, "deleted.txt", "delete me\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "src/ação.go", "staged\n")
	if err := a.StageFiles(ctx, dir, []string{"src/ação.go"}); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "src/ação.go", "unstaged\n")
	writeCommitFile(t, dir, "src/new.txt", "new\n")
	writeCommitFile(t, dir, "bin/output", "ignored\n")
	writeCommitFile(t, dir, "node_modules/large/index.js", "ignored\n")
	writeCommitFile(t, dir, ".env", "private\n")
	if err := os.Remove(filepath.Join(dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	if err := a.StageFiles(ctx, dir, []string{"deleted.txt"}); err != nil {
		t.Fatal(err)
	}
	tree, err := a.GetWorkspaceTree(ctx, filepath.Join(dir, "src"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".git", "bin", "node_modules", ".env"} {
		if findTreeNode(tree.Root, path) != nil {
			t.Fatalf("ignored path visible: %s", path)
		}
	}
	node := findTreeNode(tree.Root, "src/ação.go")
	if node == nil || !node.Status.IsStaged() || !node.Status.IsUnstaged() {
		t.Fatalf("bad staged/unstaged node: %+v", node)
	}
	if node := findTreeNode(tree.Root, "deleted.txt"); node == nil || !node.Status.IsDeleted() {
		t.Fatal("staged deletion missing from tree")
	}
	if tree.Root.StagedCount != 2 || tree.Root.UntrackedCount != 1 || tree.Workspace.Path != dir {
		t.Fatalf("bad tree %+v", tree)
	}
	if err := a.ToggleStage(ctx, dir, "src/ação.go"); err != nil {
		t.Fatal(err)
	}
	cs, err := a.GetCommitChanges(ctx, dir)
	if err != nil || cs.StagedCount() != 1 {
		t.Fatalf("unstage failed %+v: %v", cs, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "src/ação.go")); err != nil || string(data) != "unstaged\n" {
		t.Fatal("unstage discarded work")
	}
	if err := a.ToggleStage(ctx, dir, "src/new.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestApp_FileViewers_LimitsAndPaths(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "space ação.go", "first\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "space ação.go", "second\n")
	diff, err := a.GetFileDiff(ctx, dir, "space ação.go")
	if err != nil || !strings.Contains(diff.Staged, "+first") || !strings.Contains(diff.Unstaged, "+second") {
		t.Fatalf("wrong diff %+v: %v", diff, err)
	}
	writeCommitFile(t, dir, "binary.dat", "prefix\x00suffix")
	file, err := a.GetFileContent(ctx, dir, "binary.dat")
	if err != nil || !file.Binary || file.Text != "" {
		t.Fatalf("binary %+v: %v", file, err)
	}
	writeCommitFile(t, dir, "huge.txt", strings.Repeat("a", app.FilePreviewLimit-1)+"ação"+strings.Repeat("b", 100))
	file, err = a.GetFileContent(ctx, dir, "huge.txt")
	if err != nil || !file.Truncated || file.Binary || !utf8.ValidString(file.Text) || len(file.Text) > app.FilePreviewLimit {
		t.Fatalf("size limit %+v: %v", file, err)
	}
	for _, path := range []string{"../outside", filepath.Join(dir, "space ação.go"), ".git/config"} {
		if _, err := a.GetFileContent(ctx, dir, path); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
		if _, err := a.GetFileDiff(ctx, dir, path); err == nil {
			t.Fatalf("accepted unsafe diff path %q", path)
		}
	}
	outside := t.TempDir()
	writeCommitFile(t, outside, "secret", "never read\n")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetFileContent(ctx, dir, "link/secret"); err == nil {
		t.Fatal("followed an escaping symlink")
	}
	file, err = a.GetFileContent(ctx, dir, "link")
	if err != nil || file.Text != "" || !strings.Contains(file.Notice, "Link simbólico") {
		t.Fatalf("symlink %+v: %v", file, err)
	}
}

func TestApp_TreeRenameAndTogglePreservesWorkingFiles(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "old name.txt", "unchanged content\n")
	writeCommitFile(t, dir, "unrelated.txt", "keep\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "old name.txt"), filepath.Join(dir, "new ação.txt")); err != nil {
		t.Fatal(err)
	}
	if err := a.StageFiles(ctx, dir, []string{"old name.txt", "new ação.txt"}); err != nil {
		t.Fatal(err)
	}
	tree, err := a.GetWorkspaceTree(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	node := findTreeNode(tree.Root, "new ação.txt")
	if node == nil || node.Status.Staged != "R" || node.Status.OrigPath != "old name.txt" || tree.Changes.Summary().Total != 1 || tree.Root.FileCount() != 2 {
		t.Fatalf("rename metadata: %+v", node)
	}
	if err := a.ToggleStage(ctx, dir, "new ação.txt"); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Exec(ctx, dir, "diff", "--cached", "--name-only")
	if err != nil || staged != "" {
		t.Fatalf("rename unstage left partial index: %q, %v", staged, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "new ação.txt"))
	if err != nil || string(data) != "unchanged content\n" {
		t.Fatal("rename unstage changed the working file")
	}
}

func TestApp_ReviewedCommitRejectsChangedIndex(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "file.txt", "reviewed\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	reviewed, err := a.GetCommitChanges(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "file.txt", "not reviewed\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateReviewedCommit(ctx, dir, "fix: review", reviewed.StagedDiff); err == nil || !strings.Contains(err.Error(), "stage mudou") {
		t.Fatalf("changed index accepted: %v", err)
	}
	if _, err := r.RevParseHead(ctx, dir); err == nil {
		t.Fatal("unreviewed commit created")
	}
}
