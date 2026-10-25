package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func seedPull(t *testing.T) (*app.App, *git.NativeRunner, string, string) {
	t.Helper()
	a, r, dir, remote := setupPushApp(t)
	target, e := a.GetPushTarget(context.Background(), dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.PushChanges(context.Background(), dir, domain.PushOptions{Target: *target}); e != nil {
		t.Fatal(e)
	}
	return a, r, dir, remote
}
func advanceRemote(t *testing.T, a *app.App, r *git.NativeRunner, dir, path, content string) string {
	t.Helper()
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "remote-writer")
	url, e := r.Exec(ctx, dir, "remote", "get-url", "origin")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Exec(ctx, dir, "clone", "--no-hardlinks", "--", strings.TrimSpace(url), other); e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"config", "user.name", "Pull Test"}, {"config", "user.email", "pull@example.test"}, {"config", "commit.gpgsign", "false"}, {"config", "core.hooksPath", filepath.Join(other, "disabled-hooks")}} {
		if _, e = r.Exec(ctx, other, args...); e != nil {
			t.Fatal(e)
		}
	}
	writeCommitFile(t, other, path, content)
	if _, e := r.Exec(ctx, other, "add", "-f", "--", path); e != nil {
		t.Fatal(e)
	}
	oid, e := r.Commit(ctx, other, "feat: remote change")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Push(ctx, other, "origin", "main", "main", false); e != nil {
		t.Fatal(e)
	}
	return oid
}
func assertNoPullRef(t *testing.T, r *git.NativeRunner, dir string) {
	t.Helper()
	out, e := r.Exec(context.Background(), dir, "for-each-ref", "--format=%(refname)", "refs/vorq/incoming/")
	if e != nil || out != "" {
		t.Fatalf("temporary ref leaked: %q %v", out, e)
	}
}
func TestPullFastForwardAndPreviewDoesNotFetch(t *testing.T) {
	a, r, dir, _ := seedPull(t)
	ctx := context.Background()
	old, _ := r.RevParseHead(ctx, dir)
	for _, args := range [][]string{{"config", "fetch.prune", "true"}, {"config", "fetch.pruneTags", "true"}, {"update-ref", "refs/remotes/origin/local-only", old}, {"tag", "local-only", old}} {
		if _, e := r.Exec(ctx, dir, args...); e != nil {
			t.Fatal(e)
		}
	}
	newHead := advanceRemote(t, a, r, dir, "remote.txt", "new remote\n")
	tracking, _ := r.Exec(ctx, dir, "rev-parse", "refs/remotes/origin/main")
	target, e := a.GetPullTarget(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := r.Exec(ctx, dir, "rev-parse", "refs/remotes/origin/main")
	if tracking != after || strings.TrimSpace(after) != old {
		t.Fatal("preview fetched")
	}
	if _, e = a.PullChanges(ctx, dir, *target); e != nil {
		t.Fatal(e)
	}
	got, _ := r.RevParseHead(ctx, dir)
	if got != newHead {
		t.Fatalf("HEAD %s != %s", got, newHead)
	}
	cs, e := a.GetCommitChanges(ctx, dir)
	if e != nil || !cs.IsClean() {
		t.Fatal("pull dirtied workspace")
	}
	assertNoPullRef(t, r, dir)
	for _, ref := range []string{"refs/remotes/origin/local-only", "refs/tags/local-only"} {
		if _, e := r.Exec(ctx, dir, "show-ref", "--verify", ref); e != nil {
			t.Fatalf("configured prune removed %s", ref)
		}
	}
}
func TestPullRejectsDirtyAndUnreviewedState(t *testing.T) {
	for _, kind := range []string{"dirty", "untracked", "head", "source", "unreviewed"} {
		t.Run(kind, func(t *testing.T) {
			a, r, dir, _ := seedPull(t)
			ctx := context.Background()
			target, e := a.GetPullTarget(ctx, dir)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "dirty":
				writeCommitFile(t, dir, "file.txt", "dirty\n")
				_, _ = r.Exec(ctx, dir, "config", "merge.autostash", "true")
			case "untracked":
				writeCommitFile(t, dir, "new.txt", "preserve\n")
			case "head":
				writeCommitFile(t, dir, "file.txt", "commit\n")
				_ = r.Add(ctx, dir, ".")
				_, _ = r.Commit(ctx, dir, "fix: local")
			case "source":
				_, _ = r.Exec(ctx, dir, "remote", "set-url", "origin", t.TempDir())
			case "unreviewed":
				target = &domain.PullTarget{}
			}
			before, _ := r.RevParseHead(ctx, dir)
			status, _ := r.Status(ctx, dir)
			if _, e = a.PullChanges(ctx, dir, *target); e == nil {
				t.Fatal("unsafe pull accepted")
			}
			head, _ := r.RevParseHead(ctx, dir)
			after, _ := r.Status(ctx, dir)
			if before != head || status != after {
				t.Fatal("rejected pull changed files/HEAD")
			}
			stash, _ := r.Exec(ctx, dir, "stash", "list")
			if stash != "" {
				t.Fatal("unexpected stash")
			}
			assertNoPullRef(t, r, dir)
		})
	}
}
func TestPullRejectsDivergenceAndPreservesIgnoredCollision(t *testing.T) {
	for _, kind := range []string{"divergence", "ignored"} {
		t.Run(kind, func(t *testing.T) {
			a, r, dir, _ := seedPull(t)
			ctx := context.Background()
			if kind == "ignored" {
				writeCommitFile(t, dir, ".gitignore", ".env\n")
				_ = r.Add(ctx, dir, ".")
				_, _ = r.Commit(ctx, dir, "chore: ignore env")
				p, _ := a.GetPushTarget(ctx, dir)
				if _, e := a.PushChanges(ctx, dir, domain.PushOptions{Target: *p}); e != nil {
					t.Fatal(e)
				}
				_ = advanceRemote(t, a, r, dir, ".env", "remote tracked\n")
				writeCommitFile(t, dir, ".env", "private local\n")
			} else {
				_ = advanceRemote(t, a, r, dir, "remote.txt", "remote\n")
				writeCommitFile(t, dir, "local.txt", "local\n")
				_ = r.Add(ctx, dir, ".")
				_, _ = r.Commit(ctx, dir, "fix: diverged")
			}
			head, _ := r.RevParseHead(ctx, dir)
			target, e := a.GetPullTarget(ctx, dir)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = a.PullChanges(ctx, dir, *target); e == nil {
				t.Fatal("unsafe fast-forward accepted")
			}
			after, _ := r.RevParseHead(ctx, dir)
			if after != head {
				t.Fatal("HEAD moved on rejected pull")
			}
			if kind == "ignored" {
				raw, _ := os.ReadFile(filepath.Join(dir, ".env"))
				if string(raw) != "private local\n" {
					t.Fatal("ignored bytes lost")
				}
			}
			if _, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD"); e == nil {
				t.Fatal("merge started")
			}
			assertNoPullRef(t, r, dir)
		})
	}
}

type interruptedPull struct {
	*git.Repository
	mutate func()
}

func (r *interruptedPull) FetchPull(ctx context.Context, dir, remote, branch string) (string, string, error) {
	hash, ref, e := r.Repository.FetchPull(ctx, dir, remote, branch)
	if e == nil {
		r.mutate()
	}
	return hash, ref, e
}
func TestPullRechecksStatusAfterFetch(t *testing.T) {
	a, r, dir, _ := seedPull(t)
	ctx := context.Background()
	_ = advanceRemote(t, a, r, dir, "remote.txt", "remote\n")
	wrapped := &interruptedPull{Repository: git.NewRepository(r), mutate: func() { writeCommitFile(t, dir, "concurrent.txt", "preserve\n") }}
	a = app.NewApp(wrapped)
	target, e := a.GetPullTarget(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	head, _ := r.RevParseHead(ctx, dir)
	if _, e = a.PullChanges(ctx, dir, *target); e == nil || !strings.Contains(e.Error(), "aplicação bloqueada") {
		t.Fatal(e)
	}
	after, _ := r.RevParseHead(ctx, dir)
	if after != head {
		t.Fatal("HEAD advanced after concurrent edits")
	}
	assertNoPullRef(t, r, dir)
}
func TestPullRequiresUpstreamAndAttachedBranch(t *testing.T) {
	a, r, dir, _ := setupPushApp(t)
	ctx := context.Background()
	if _, e := a.GetPullTarget(ctx, dir); e == nil {
		t.Fatal("untracked branch accepted")
	}
	_, _ = r.Exec(ctx, dir, "switch", "--detach")
	if _, e := a.GetPullTarget(ctx, dir); e == nil {
		t.Fatal("detached accepted")
	}
}
