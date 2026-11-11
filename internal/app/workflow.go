package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type WorkflowRepository interface {
	RemoteWebURL(context.Context, string, string) (string, string, error)
	ValidBranchName(context.Context, string, string) bool
	ResolveCommit(context.Context, string, string) (string, error)
	CreateBranchAt(context.Context, string, string, string, bool) error
	History(context.Context, string, domain.HistoryOptions) (*domain.HistoryPage, error)
	CommitDetail(context.Context, string, string) (*domain.CommitDetail, error)
	RemoteDefaultBranch(context.Context, string, string) (string, error)
	CommitDraft(context.Context, string, string, string) (string, string, error)
	PublishedHead(context.Context, string, string, string, string) error
	BranchChanges(context.Context, string, string, string) (*domain.BranchChanges, error)
}
type PullRequestProvider interface {
	List(context.Context, string, string, domain.PRListOptions) ([]domain.PullRequest, error)
	View(context.Context, string, string, int) (*domain.PullRequest, error)
	Save(context.Context, string, domain.PRDraft) (string, error)
	DefaultBranch(context.Context, string, string) (string, error)
	Ready(context.Context, string, string, int) error
	// MergeInfo/Merge (VORQ-020): Merge is pinned to the reviewed head commit
	// and never bypasses protections (no --admin, --auto or --delete-branch).
	MergeInfo(context.Context, string, string, int) (*domain.PRMergeInfo, error)
	Merge(context.Context, string, string, int, string, string) error
}
type WorkflowService interface {
	GetRepositoryLink(context.Context, string) (*domain.RepositoryLink, error)
	CreateBranch(context.Context, string, domain.BranchOptions) error
	GetHistory(context.Context, string, domain.HistoryOptions) (*domain.HistoryPage, error)
	GetCommitDetail(context.Context, string, string) (*domain.CommitDetail, error)
	ListPullRequests(context.Context, string, domain.PRListOptions) ([]domain.PullRequest, error)
	ViewPullRequest(context.Context, string, int) (*domain.PullRequest, error)
	PreparePullRequest(context.Context, string, string) (*domain.PRDraft, error)
	SavePullRequest(context.Context, string, domain.PRDraft) (string, error)
	MarkPullRequestReady(context.Context, string, int) error
}

func (a *App) SetPullRequestProvider(p PullRequestProvider) { a.pullRequests = p }

// selectRemote is the single remote policy (VORQ-017) shared by the
// repository link, PRs, spawn defaults and push without upstream: the
// branch's upstream remote, else origin, else the only remote; otherwise none.
func selectRemote(remotes []string, upstream string) string {
	has := func(name string) bool {
		for _, r := range remotes {
			if r == name {
				return true
			}
		}
		return false
	}
	switch {
	case upstream != "" && has(upstream):
		return upstream
	case has("origin"):
		return "origin"
	case len(remotes) == 1:
		return remotes[0]
	}
	return ""
}

func (a *App) GetRepositoryLink(ctx context.Context, dir string) (*domain.RepositoryLink, error) {
	remotes, e := a.gitRunner.Remotes(ctx, dir)
	if e != nil {
		return nil, e
	}
	link := &domain.RepositoryLink{}
	if len(remotes) == 0 {
		return link, nil
	}
	branch, detached, e := a.gitRunner.Branch(ctx, dir)
	if e != nil {
		return nil, e
	}
	upstream := ""
	if !detached {
		exists, e := a.gitRunner.BranchExists(ctx, dir, branch)
		if e != nil {
			return nil, e
		}
		if exists {
			if upstream, _, e = a.gitRunner.UpstreamTarget(ctx, dir, branch); e != nil {
				return nil, e
			}
		}
	}
	if link.Remote = selectRemote(remotes, upstream); link.Remote == "" {
		return link, nil
	}
	link.URL, link.Slug, e = a.gitRunner.RemoteWebURL(ctx, dir, link.Remote)
	if e != nil {
		return nil, e
	}
	return link, nil
}

// CreateBranch owns the branch rules: a valid new name, an existing first
// commit, and a clean workspace when entering a different base (the
// current HEAD keeps working files and stage as they are).
func (a *App) CreateBranch(ctx context.Context, dir string, o domain.BranchOptions) error {
	if o.Name == "" || strings.HasPrefix(o.Name, "-") || o.Name == "HEAD" || !a.gitRunner.ValidBranchName(ctx, dir, o.Name) {
		return fmt.Errorf("invalid branch name")
	}
	exists, e := a.gitRunner.BranchExists(ctx, dir, o.Name)
	if e != nil {
		return e
	}
	if exists {
		return fmt.Errorf("branch %q already exists", o.Name)
	}
	head, e := a.gitRunner.RevParseHead(ctx, dir)
	if e != nil {
		return fmt.Errorf("create the first commit before creating another branch: %w", e)
	}
	base := head
	if o.Base != "" {
		if base, e = a.gitRunner.ResolveCommit(ctx, dir, o.Base); e != nil {
			return fmt.Errorf("invalid base: %w", e)
		}
	}
	if !o.Stay && base != head {
		changes, e := a.gitRunner.ReadChanges(ctx, dir, "")
		if e != nil {
			return e
		}
		if !changes.IsClean() {
			return fmt.Errorf("another base requires a clean workspace; stage and files were preserved")
		}
	}
	return a.gitRunner.CreateBranchAt(ctx, dir, o.Name, base, !o.Stay)
}

func (a *App) GetHistory(ctx context.Context, dir string, opts domain.HistoryOptions) (*domain.HistoryPage, error) {
	if opts.Skip < 0 || opts.Limit < 0 || opts.Limit > 500 {
		return nil, fmt.Errorf("history: offset must be >= 0 and limit between 1 and 500")
	}
	if opts.Limit == 0 {
		opts.Limit = 100
	}
	r := a.gitRunner
	return r.History(ctx, dir, opts)
}
func (a *App) GetCommitDetail(ctx context.Context, dir, hash string) (*domain.CommitDetail, error) {
	if len(hash) != 40 && len(hash) != 64 {
		return nil, fmt.Errorf("invalid commit hash")
	}
	for _, c := range hash {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return nil, fmt.Errorf("invalid commit hash")
		}
	}
	r := a.gitRunner
	return r.CommitDetail(ctx, dir, hash)
}
func (a *App) prRepository(ctx context.Context, dir string) (*domain.RepositoryLink, error) {
	if a.pullRequests == nil {
		return nil, fmt.Errorf("PRs require gh installed and authenticated (gh auth login)")
	}
	link, e := a.GetRepositoryLink(ctx, dir)
	if e != nil {
		return nil, e
	}
	if link.URL == "" {
		return nil, fmt.Errorf("select an unambiguous GitHub remote")
	}
	u, e := url.Parse(link.URL)
	if e != nil || u.Host != "github.com" {
		return nil, fmt.Errorf("PRs in this version require a github.com remote")
	}
	return link, nil
}
func (a *App) ListPullRequests(ctx context.Context, dir string, o domain.PRListOptions) ([]domain.PullRequest, error) {
	if o.State == "" {
		o.State = "open"
	}
	if o.State != "all" && o.State != "open" && o.State != "closed" && o.State != "merged" {
		return nil, fmt.Errorf("invalid PR state")
	}
	if o.Limit == 0 {
		o.Limit = 100
	}
	if o.Limit < 1 || o.Limit > 500 {
		return nil, fmt.Errorf("PR limit must be between 1 and 500")
	}
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	return a.pullRequests.List(ctx, dir, link.Slug, o)
}
func (a *App) ViewPullRequest(ctx context.Context, dir string, number int) (*domain.PullRequest, error) {
	if number <= 0 {
		return nil, fmt.Errorf("invalid PR number")
	}
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	return a.pullRequests.View(ctx, dir, link.Slug, number)
}
func (a *App) PreparePullRequest(ctx context.Context, dir, base string) (*domain.PRDraft, error) {
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	r := a.gitRunner
	head, detached, e := a.gitRunner.Branch(ctx, dir)
	if e != nil {
		return nil, e
	}
	if detached {
		return nil, fmt.Errorf("create a branch before opening a PR")
	}
	if base == "" {
		base, e = r.RemoteDefaultBranch(ctx, dir, link.Remote)
		if e != nil {
			return nil, e
		}
		if base == "" {
			base, e = a.pullRequests.DefaultBranch(ctx, dir, link.Slug)
			if e != nil {
				return nil, e
			}
		}
	}
	if head == base {
		return nil, fmt.Errorf("the base must differ from the current branch")
	}
	title, body, e := r.CommitDraft(ctx, dir, link.Remote, base)
	if e != nil {
		return nil, e
	}
	oid, e := a.gitRunner.RevParseHead(ctx, dir)
	if e != nil {
		return nil, e
	}
	return &domain.PRDraft{Repository: link.Slug, Head: head, HeadOID: oid, Base: base, Title: title, Body: body, Draft: true}, nil
}

// MarkPullRequestReady turns an open draft PR into one ready for review, so
// GitHub allows merging it. It checks the current state first; never merges.
func (a *App) MarkPullRequestReady(ctx context.Context, dir string, number int) error {
	if number <= 0 {
		return fmt.Errorf("invalid PR number")
	}
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return e
	}
	pr, e := a.pullRequests.View(ctx, dir, link.Slug, number)
	if e != nil {
		return e
	}
	if !strings.EqualFold(pr.State, "open") {
		return fmt.Errorf("only open PRs can be marked ready (PR #%d is %s)", number, strings.ToLower(pr.State))
	}
	if !pr.Draft {
		return fmt.Errorf("PR #%d is already ready for review", number)
	}
	return a.pullRequests.Ready(ctx, dir, link.Slug, number)
}

func (a *App) SavePullRequest(ctx context.Context, dir string, d domain.PRDraft) (string, error) {
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return "", e
	}
	if d.Repository != link.Slug {
		return "", fmt.Errorf("destination changed; review the PR again")
	}
	if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Base) == "" {
		return "", fmt.Errorf("title and base are required")
	}
	if d.Number < 0 {
		return "", fmt.Errorf("invalid PR number")
	}
	if d.Number == 0 {
		branch, detached, e := a.gitRunner.Branch(ctx, dir)
		if e != nil {
			return "", e
		}
		oid, e := a.gitRunner.RevParseHead(ctx, dir)
		if e != nil {
			return "", e
		}
		if detached || branch != d.Head || oid != d.HeadOID {
			return "", fmt.Errorf("branch/HEAD changed; prepare the PR again")
		}
		if d.Head == d.Base {
			return "", fmt.Errorf("base and head are the same")
		}
		r := a.gitRunner
		if e = r.PublishedHead(ctx, dir, link.Remote, d.Head, d.HeadOID); e != nil {
			return "", e
		}
	}
	return a.pullRequests.Save(ctx, dir, d)
}
