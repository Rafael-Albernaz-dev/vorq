package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func TestNativeRunner_Commit_HeadReadFailurePreservesCreatedOutcome(t *testing.T) {
	native, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "real.txt", "real data\n")
	if err := native.Add(ctx, dir, "real.txt"); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	wrapperDir := t.TempDir()
	// A test-only executable delegates the real commit but fails only the
	// subsequent HEAD read. The repository and every commit remain in t.TempDir.
	wrapper := `#!/bin/sh
if [ "$1" = "rev-parse" ] && [ "$2" = "--verify" ] && [ "$3" = "HEAD" ]; then
 printf 'HEAD metadata read unavailable\n' >&2
 exit 75
fi
exec "$VORQ_TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(wrapperDir, "git"), []byte(wrapper), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VORQ_TEST_REAL_GIT", realGit)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := runner.Commit(ctx, dir, "feat: created once")
	var outcome *domain.CommitMetadataError
	if hash != "" || !errors.As(err, &outcome) || outcome.Hash != "" || outcome.Operation != "resolving HEAD" {
		t.Fatalf("lost created outcome: hash %q err %v", hash, err)
	}
	if !strings.Contains(err.Error(), "commit created") {
		t.Fatalf("ambiguous error: %v", err)
	}
	actual, headErr := native.RevParseHead(ctx, dir)
	if headErr != nil || len(actual) != 40 {
		t.Fatalf("native commit was not created: %q %v", actual, headErr)
	}
	count, countErr := native.Exec(ctx, dir, "rev-list", "--count", "HEAD")
	if countErr != nil || strings.TrimSpace(count) != "1" {
		t.Fatalf("commit repeated: %q %v", count, countErr)
	}
}
