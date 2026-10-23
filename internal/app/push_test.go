package app_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func setupPushApp(t *testing.T) (*app.App, *git.NativeRunner, string, string) {
	t.Helper()
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "file.txt", "initial\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial"); err != nil {
		t.Fatal(err)
	}
	remote := t.TempDir()
	runCmd(t, remote, "git", "init", "--bare", "-b", "main")
	runCmd(t, dir, "git", "remote", "add", "origin", remote)
	return a, r, dir, remote
}

func TestApp_Push_NoUpstreamThenTracked(t *testing.T) {
	a, r, dir, remote := setupPushApp(t)
	ctx := context.Background()
	target, err := a.GetPushTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.HasUpstream || target.Remote != "origin" || target.Branch != "main" || target.RemoteBranch != "main" || target.DestinationID == "" {
		t.Fatalf("target %+v", target)
	}
	if _, err := r.Exec(ctx, remote, "show-ref", "--verify", "refs/heads/main"); err == nil {
		t.Fatal("preview published a branch")
	}
	if _, err := a.PushChanges(ctx, dir, domain.PushOptions{}); err == nil {
		t.Fatal("unreviewed push accepted")
	}
	if _, err := a.PushChanges(ctx, dir, domain.PushOptions{Target: *target}); err != nil {
		t.Fatal(err)
	}
	upstream, err := r.Upstream(ctx, dir)
	if err != nil || upstream != "origin/main" {
		t.Fatalf("upstream %q: %v", upstream, err)
	}
	target, err = a.GetPushTarget(ctx, dir)
	if err != nil || !target.HasUpstream || target.Ahead != 0 {
		t.Fatalf("target %+v: %v", target, err)
	}
	remoteHead, err := r.RevParseHead(ctx, remote)
	if err != nil || remoteHead != target.HeadHash {
		t.Fatalf("remote HEAD %s: %v", remoteHead, err)
	}
}

func TestApp_Push_DifferentUpstreamBranch(t *testing.T) {
	a, r, dir, remote := setupPushApp(t)
	ctx := context.Background()
	if _, err := r.Push(ctx, dir, "origin", "main", "published/release", true); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "file.txt", "next\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	result, err := a.CreateCommit(ctx, dir, "fix: next")
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.GetPushTarget(ctx, dir)
	if err != nil || target.RemoteBranch != "published/release" || target.Ahead != 1 {
		t.Fatalf("target %+v: %v", target, err)
	}
	if _, err := a.PushChanges(ctx, dir, domain.PushOptions{Target: *target}); err != nil {
		t.Fatal(err)
	}
	out, err := r.Exec(ctx, remote, "rev-parse", "refs/heads/published/release")
	if err != nil || strings.TrimSpace(out) != result.Hash {
		t.Fatalf("wrong destination %q: %v", out, err)
	}
	if _, err := r.Exec(ctx, remote, "show-ref", "--verify", "refs/heads/main"); err == nil {
		t.Fatal("unexpected branch main published")
	}
}

func TestApp_Push_RejectsStalePreview(t *testing.T) {
	for _, change := range []string{"head", "destination"} {
		t.Run(change, func(t *testing.T) {
			a, _, dir, _ := setupPushApp(t)
			ctx := context.Background()
			target, err := a.GetPushTarget(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			if change == "head" {
				writeCommitFile(t, dir, "file.txt", "changed\n")
				if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
					t.Fatal(err)
				}
				if _, err := a.CreateCommit(ctx, dir, "fix: changed"); err != nil {
					t.Fatal(err)
				}
			} else {
				newRemote := t.TempDir()
				runCmd(t, newRemote, "git", "init", "--bare")
				runCmd(t, dir, "git", "remote", "set-url", "--push", "origin", newRemote)
			}
			if _, err := a.PushChanges(ctx, dir, domain.PushOptions{Target: *target}); err == nil || !strings.Contains(err.Error(), "target changed") {
				t.Fatalf("stale push accepted: %v", err)
			}
		})
	}
}

func TestApp_Push_RejectsNonFastForward(t *testing.T) {
	a, r, dir, remote := setupPushApp(t)
	ctx := context.Background()
	target, err := a.GetPushTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.PushChanges(ctx, dir, domain.PushOptions{Target: *target}); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "other-worktree")
	runCmd(t, dir, "git", "worktree", "add", "-b", "competitor", other, "main")
	writeCommitFile(t, other, "file.txt", "remote diverged\n")
	if err := a.StageFiles(ctx, other, []string{"."}); err != nil {
		t.Fatal(err)
	}
	otherCommit, err := a.CreateCommit(ctx, other, "fix: competitor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Push(ctx, other, "origin", "competitor", "main", false); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "file.txt", "local diverged\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "fix: local"); err != nil {
		t.Fatal(err)
	}
	target, err = a.GetPushTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.PushChanges(ctx, dir, domain.PushOptions{Target: *target}); err == nil {
		t.Fatal("non-fast-forward was not rejected")
	}
	out, err := r.Exec(ctx, remote, "rev-parse", "refs/heads/main")
	if err != nil || strings.TrimSpace(out) != otherCommit.Hash {
		t.Fatal("remote history was overwritten")
	}
}

func TestApp_Push_ConfigurationErrors(t *testing.T) {
	a, _, dir, _ := setupPushApp(t)
	ctx := context.Background()
	runCmd(t, dir, "git", "remote", "remove", "origin")
	if _, err := a.GetPushTarget(ctx, dir); err == nil {
		t.Fatal("accepted no remotes")
	}
	runCmd(t, dir, "git", "remote", "add", "alpha", t.TempDir())
	runCmd(t, dir, "git", "remote", "add", "beta", t.TempDir())
	if _, err := a.GetPushTarget(ctx, dir); err == nil {
		t.Fatal("guessed ambiguous remote")
	}
	runCmd(t, dir, "git", "checkout", "--detach")
	if _, err := a.GetPushTarget(ctx, dir); err == nil || !strings.Contains(err.Error(), "detached HEAD") {
		t.Fatalf("detached push: %v", err)
	}
}

func TestCLI_Push(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "vorq")
	if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/vorq").CombinedOutput(); err != nil {
		t.Fatalf("build %v: %s", err, out)
	}
	for _, tt := range []struct {
		name, input        string
		args               []string
		published, wantErr bool
	}{
		{"cancel", "n\n", nil, false, false},
		{"eof", "", nil, false, true},
		{"confirm", "\n", nil, true, false},
		{"yes", "", []string{"--yes"}, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, r, dir, remote := setupPushApp(t)
			cmd := exec.Command(binary, append([]string{"push"}, tt.args...)...)
			cmd.Dir = dir
			cmd.Stdin = strings.NewReader(tt.input)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tt.wantErr || !strings.Contains(string(out), "Local: main -> Remote: origin/main") {
				t.Fatalf("CLI err=%v: %s", err, out)
			}
			_, err = r.Exec(context.Background(), remote, "show-ref", "--verify", "refs/heads/main")
			if (err == nil) != tt.published {
				t.Fatalf("unexpected remote mutation: %v", err)
			}
		})
	}
}
