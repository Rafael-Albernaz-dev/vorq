package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func TestProjectPathsPreserveTrailingWhitespace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo with trailing space ")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := r.Exec(ctx, dir, "init", "-q", "-b", "main"); err != nil {
		t.Fatal(err)
	}
	root, common, err := git.NewRepository(r).ProjectPaths(ctx, dir)
	if err != nil || root != dir || common != filepath.Join(dir, ".git") {
		t.Fatalf("path altered: %q %q %v", root, common, err)
	}
}

func TestRepositoryInspectionErrorsAreNotReportedAsMissingUpstreamOrBranch(t *testing.T) {
	r, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx := context.Background()
	if _, _, err := r.AheadBehind(ctx, dir); err == nil {
		t.Fatal("invalid repository reported as synchronised")
	}
	if _, err := r.BranchExists(ctx, dir, "main"); err == nil {
		t.Fatal("repository failure reported as missing branch")
	}
}
