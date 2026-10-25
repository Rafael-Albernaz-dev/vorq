package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func seedWorkflow(t *testing.T) (*app.App, *git.NativeRunner, string) {
	t.Helper()
	a, r, dir := setupCommitApp(t)
	writeCommitFile(t, dir, "file.txt", "initial\n")
	if e := r.Add(context.Background(), dir, "."); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Commit(context.Background(), dir, "feat: initial"); e != nil {
		t.Fatal(e)
	}
	return a, r, dir
}
func TestCreateBranchPreservesIndexWorkingFilesAndIgnoredData(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "file.txt", "staged\n")
	if e := r.Add(ctx, dir, "file.txt"); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, "file.txt", "working\n")
	writeCommitFile(t, dir, ".gitignore", ".env\n")
	writeCommitFile(t, dir, ".env", "private\n")
	writeCommitFile(t, dir, "untracked.txt", "untracked\n")
	before, e := r.Status(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	index, e := r.DiffCached(ctx, dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feat/new"}); e != nil {
		t.Fatal(e)
	}
	branch, e := r.CurrentBranch(ctx, dir)
	if e != nil || branch != "feat/new" {
		t.Fatalf("branch %s: %v", branch, e)
	}
	after, _ := r.Status(ctx, dir)
	staged, _ := r.DiffCached(ctx, dir)
	if after != before || staged != index {
		t.Fatal("stage/status changed")
	}
	for name, want := range map[string]string{"file.txt": "working\n", ".env": "private\n", "untracked.txt": "untracked\n"} {
		got, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil || string(got) != want {
			t.Fatalf("lost %s: %v", name, e)
		}
	}
	for _, name := range []string{"feat/new", "-f", "HEAD", "a..b", "bad name", "@{-1}"} {
		if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: name}); e == nil {
			t.Fatalf("accepted invalid/colliding %s", name)
		}
	}
}
func TestCreateBranchOtherBaseDirtyAndDetached(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	old, _ := r.RevParseHead(ctx, dir)
	writeCommitFile(t, dir, "file.txt", "second\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "feat: second")
	writeCommitFile(t, dir, "file.txt", "dirty\n")
	if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "blocked", Base: old}); e == nil {
		t.Fatal("other base with dirty files accepted")
	}
	exists, _ := r.BranchExists(ctx, dir, "blocked")
	if exists {
		t.Fatal("branch leaked on rejection")
	}
	if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "stay", Base: old, Stay: true}); e != nil {
		t.Fatal(e)
	}
	branch, _ := r.CurrentBranch(ctx, dir)
	if branch != "main" {
		t.Fatal("stay switched")
	}
	// Discard is explicit only inside this isolated test fixture.
	if _, e := r.Exec(ctx, dir, "restore", "--worktree", "--", "file.txt"); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Exec(ctx, dir, "switch", "--detach", old); e != nil {
		t.Fatal(e)
	}
	if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "from-detached"}); e != nil {
		t.Fatal(e)
	}
}
func TestHistoryMergePaginationSearchAndDetails(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	_ = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feature"})
	writeCommitFile(t, dir, "feature.txt", "feature\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "feat: branch commit")
	if _, e := r.Exec(ctx, dir, "switch", "main"); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, "main.txt", "main\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "fix: main commit")
	if _, e := r.Exec(ctx, dir, "merge", "--no-ff", "feature", "-m", "Merge feature"); e != nil {
		t.Fatal(e)
	}
	_, _ = r.Exec(ctx, dir, "tag", "v-test")
	seen := map[string]bool{}
	snapshot := ""
	merges := 0
	for skip := 0; skip < 10; {
		p, e := a.GetHistory(ctx, dir, domain.HistoryOptions{Limit: 2, Skip: skip})
		if e != nil {
			t.Fatal(e)
		}
		if snapshot != "" && p.Snapshot != snapshot {
			t.Fatal("unstable snapshot")
		}
		snapshot = p.Snapshot
		for _, l := range p.Lines {
			if l.Hash == "" {
				continue
			}
			if seen[l.Hash] {
				t.Fatal("duplicate across pages")
			}
			seen[l.Hash] = true
			if len(strings.Fields(l.Parents)) == 2 {
				merges++
				d, e := a.GetCommitDetail(ctx, dir, l.Hash)
				if e != nil || !strings.Contains(d.Text, "feature.txt") {
					t.Fatalf("merge details: %v %v", d, e)
				}
			}
		}
		skip += p.Count
		if !p.HasMore {
			break
		}
	}
	if len(seen) != 4 || merges != 1 {
		t.Fatalf("history incomplete: %d / merges %d", len(seen), merges)
	}
	p, e := a.GetHistory(ctx, dir, domain.HistoryOptions{Limit: 10, Search: "branch commit", Ref: "feature"})
	if e != nil || p.Count != 1 {
		t.Fatalf("filter: %+v %v", p, e)
	}
	if _, e := a.GetCommitDetail(ctx, dir, "--help"); e == nil {
		t.Fatal("accepted unsafe revision")
	}
}

type prRepository struct {
	*git.Repository
	published bool
	checks    int
}

func (r *prRepository) PublishedHead(context.Context, string, string, string, string) error {
	r.checks++
	if !r.published {
		return fmt.Errorf("publish first")
	}
	return nil
}

type fakePRProvider struct{ saved []domain.PRDraft }

func (f *fakePRProvider) List(context.Context, string, string, domain.PRListOptions) ([]domain.PullRequest, error) {
	return nil, nil
}
func (f *fakePRProvider) View(context.Context, string, string, int) (*domain.PullRequest, error) {
	return nil, nil
}
func (f *fakePRProvider) Save(_ context.Context, _ string, d domain.PRDraft) (string, error) {
	f.saved = append(f.saved, d)
	return "https://github.com/owner/repo/pull/1", nil
}
func (f *fakePRProvider) DefaultBranch(context.Context, string, string) (string, error) {
	return "main", nil
}
func TestPRPreparationAndSaveRequirePublishedUnchangedHead(t *testing.T) {
	_, r, dir := seedWorkflow(t)
	ctx := context.Background()
	repo := &prRepository{Repository: git.NewRepository(r)}
	a := app.NewApp(repo)
	provider := &fakePRProvider{}
	a.SetPullRequestProvider(provider)
	if _, e := r.Exec(ctx, dir, "remote", "add", "origin", "git@github.com:owner/repo.git"); e != nil {
		t.Fatal(e)
	}
	_ = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feature"})
	writeCommitFile(t, dir, "new.txt", "new\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "feat: my PR")
	d, e := a.PreparePullRequest(ctx, dir, "")
	if e != nil || d.Title != "feat: my PR" || d.Repository != "owner/repo" || !d.Draft {
		t.Fatalf("draft %+v: %v", d, e)
	}
	if _, e = a.SavePullRequest(ctx, dir, *d); e == nil || len(provider.saved) != 0 {
		t.Fatal("unpublished head sent")
	}
	repo.published = true
	changed := *d
	changed.Repository = "wrong/repo"
	if _, e = a.SavePullRequest(ctx, dir, changed); e == nil {
		t.Fatal("changed target sent")
	}
	changed = *d
	changed.HeadOID = "stale"
	if _, e = a.SavePullRequest(ctx, dir, changed); e == nil {
		t.Fatal("stale HEAD sent")
	}
	if _, e = a.SavePullRequest(ctx, dir, *d); e != nil || len(provider.saved) != 1 {
		t.Fatalf("save failed %v", e)
	}
}
func TestEmptyHistoryAndMissingGH(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	p, e := a.GetHistory(context.Background(), dir, domain.HistoryOptions{})
	if e != nil || p.Count != 0 {
		t.Fatalf("empty history %+v %v", p, e)
	}
	if _, e := a.ListPullRequests(context.Background(), dir, domain.PRListOptions{}); e == nil || !strings.Contains(e.Error(), "gh") {
		t.Fatal("missing gh not explained")
	}
}

func TestOtherBaseNeverOverwritesIgnoredFile(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", ".env\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "chore: ignore env")
	if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "with-env"}); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, ".env", "tracked on other branch\n")
	if _, e := r.Exec(ctx, dir, "add", "-f", "--", ".env"); e != nil {
		t.Fatal(e)
	}
	_, _ = r.Commit(ctx, dir, "test: tracked env fixture")
	if _, e := r.Exec(ctx, dir, "switch", "main"); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, ".env", "private local\n")
	if e := a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "must-fail", Base: "with-env"}); e == nil {
		t.Fatal("ignored file overwritten")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(raw) != "private local\n" {
		t.Fatal("ignored data lost")
	}
	branch, _ := r.CurrentBranch(ctx, dir)
	exists, _ := r.BranchExists(ctx, dir, "must-fail")
	if branch != "main" || exists {
		t.Fatal("failed switch changed branch state")
	}
}

func TestPublishedHeadUsesExactRemoteBranchAndOID(t *testing.T) {
	_, r, dir := seedWorkflow(t)
	ctx := context.Background()
	remote := t.TempDir()
	if _, e := r.Exec(ctx, remote, "init", "--bare"); e != nil {
		t.Fatal(e)
	}
	_, _ = r.Exec(ctx, dir, "remote", "add", "origin", remote)
	repo := git.NewRepository(r)
	oid, _ := r.RevParseHead(ctx, dir)
	if e := repo.PublishedHead(ctx, dir, "origin", "main", oid); e == nil {
		t.Fatal("missing remote branch accepted")
	}
	if _, e := r.Exec(ctx, dir, "push", "origin", "refs/heads/main:refs/heads/main"); e != nil {
		t.Fatal(e)
	}
	if e := repo.PublishedHead(ctx, dir, "origin", "main", oid); e != nil {
		t.Fatal(e)
	}
	if e := repo.PublishedHead(ctx, dir, "origin", "main", strings.Repeat("0", 40)); e == nil {
		t.Fatal("different remote head accepted")
	}
}

func TestRepositoryIdentitySelection(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	for _, args := range [][]string{{"remote", "add", "upstream", "git@github.com:upstream/repo.git"}, {"remote", "add", "backup", "https://github.com/backup/repo.git"}} {
		if _, e := r.Exec(ctx, dir, args...); e != nil {
			t.Fatal(e)
		}
	}
	link, e := a.GetRepositoryLink(ctx, dir)
	if e != nil || link.URL != "" {
		t.Fatalf("ambiguous remote guessed: %+v %v", link, e)
	}
	_, _ = r.Exec(ctx, dir, "remote", "add", "origin", "https://user:password@github.com/owner/repo.git?secret=x")
	link, e = a.GetRepositoryLink(ctx, dir)
	if e != nil || link.URL != "https://github.com/owner/repo" {
		t.Fatalf("origin identity %+v %v", link, e)
	}
	oid, _ := r.RevParseHead(ctx, dir)
	_, _ = r.Exec(ctx, dir, "update-ref", "refs/remotes/upstream/main", oid)
	_, _ = r.Exec(ctx, dir, "config", "branch.main.remote", "upstream")
	_, _ = r.Exec(ctx, dir, "config", "branch.main.merge", "refs/heads/main")
	link, e = a.GetRepositoryLink(ctx, dir)
	if e != nil || link.Slug != "upstream/repo" {
		t.Fatalf("upstream identity %+v %v", link, e)
	}
}

func TestShallowHistoryAndLargePatchNotice(t *testing.T) {
	a, r, dir := seedWorkflow(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "large.txt", strings.Repeat("content line\n", 30000))
	_ = r.Add(ctx, dir, ".")
	oid, e := r.Commit(ctx, dir, "feat: large text")
	if e != nil {
		t.Fatal(e)
	}
	d, e := a.GetCommitDetail(ctx, dir, oid)
	if e != nil || len(d.Text) > 270000 || !strings.Contains(d.Text, "Preview limitado") {
		t.Fatalf("large preview len=%d: %v", len(d.Text), e)
	}
	clone := filepath.Join(t.TempDir(), "shallow")
	if _, e := r.Exec(ctx, "", "clone", "--depth=1", "--no-local", "--", "file://"+dir, clone); e != nil {
		t.Fatal(e)
	}
	p, e := a.GetHistory(ctx, clone, domain.HistoryOptions{Limit: 10})
	if e != nil || !p.Shallow || p.Count != 1 {
		t.Fatalf("shallow %+v %v", p, e)
	}
}
