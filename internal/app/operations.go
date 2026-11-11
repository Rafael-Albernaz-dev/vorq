package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// OperationRepository covers cherry-pick, rebase and resuming/aborting a
// stopped Git operation (VORQ-020 C).
type OperationRepository interface {
	OperationInProgress(context.Context, string) (string, error)
	CherryPick(context.Context, string, string) (string, error)
	Rebase(context.Context, string, string) (string, error)
	ContinueOperation(context.Context, string, string) (string, error)
	AbortOperation(context.Context, string, string) error
	CommitSummary(context.Context, string, string) (string, int, error)
	PatchApplied(context.Context, string, string, string) bool
}
type OperationService interface {
	GetOperationState(context.Context, string) (*domain.OperationState, error)
	ContinueOperation(context.Context, string) (*domain.OperationResult, error)
	AbortOperation(context.Context, string) (string, error)
	PrepareCherryPick(context.Context, string, string) (*domain.CherryPickPlan, error)
	CherryPick(context.Context, string, domain.CherryPickPlan) (*domain.OperationResult, error)
	PrepareRebase(context.Context, string) (*domain.RebasePlan, error)
	Rebase(context.Context, string, domain.RebasePlan) (*domain.OperationResult, error)
}

func (a *App) GetOperationState(ctx context.Context, dir string) (*domain.OperationState, error) {
	p, e := a.ResolveProject(ctx, dir)
	if e != nil {
		return nil, e
	}
	kind, e := a.gitRunner.OperationInProgress(ctx, p.RootPath)
	if e != nil {
		return nil, e
	}
	state := &domain.OperationState{Kind: kind}
	if kind != "" {
		if state.Conflicts, e = a.gitRunner.UnmergedPaths(ctx, p.RootPath); e != nil {
			return nil, e
		}
	}
	return state, nil
}

// ContinueOperation resumes after the user resolved and staged everything.
func (a *App) ContinueOperation(ctx context.Context, dir string) (*domain.OperationResult, error) {
	state, e := a.GetOperationState(ctx, dir)
	if e != nil {
		return nil, e
	}
	switch state.Kind {
	case "":
		return nil, fmt.Errorf("no Git operation in progress")
	case "revert":
		return nil, fmt.Errorf("a revert is in progress; finish it with git revert --continue or abort it")
	}
	if len(state.Conflicts) > 0 {
		return nil, fmt.Errorf("%d file(s) still in conflict; resolve and stage them first: %s", len(state.Conflicts), strings.Join(state.Conflicts, ", "))
	}
	p, _ := a.ResolveProject(ctx, dir)
	out, err := a.gitRunner.ContinueOperation(ctx, p.RootPath, state.Kind)
	return a.operationOutcome(ctx, p.RootPath, state.Kind, out, err)
}

// AbortOperation restores the state before the stopped operation; it refuses
// when nothing is in progress, so it never discards unrelated work.
func (a *App) AbortOperation(ctx context.Context, dir string) (string, error) {
	state, e := a.GetOperationState(ctx, dir)
	if e != nil {
		return "", e
	}
	if state.Kind == "" {
		return "", fmt.Errorf("no Git operation in progress; nothing was changed")
	}
	p, _ := a.ResolveProject(ctx, dir)
	return state.Kind, a.gitRunner.AbortOperation(ctx, p.RootPath, state.Kind)
}

// operationOutcome turns a native result into done or stopped-for-resolution.
func (a *App) operationOutcome(ctx context.Context, root, kind, out string, runErr error) (*domain.OperationResult, error) {
	result := &domain.OperationResult{Kind: kind, Output: out}
	if runErr == nil {
		if still, _ := a.gitRunner.OperationInProgress(ctx, root); still == "" {
			return result, nil
		}
	}
	still, e := a.gitRunner.OperationInProgress(ctx, root)
	if e != nil {
		return nil, e
	}
	if still == "" {
		return nil, fmt.Errorf("%s not applied; HEAD and files preserved: %w", kind, runErr)
	}
	result.Stopped = true
	if runErr != nil {
		result.Output = runErr.Error()
	}
	result.Conflicts, e = a.gitRunner.UnmergedPaths(ctx, root)
	return result, e
}

// requireIdleBranch: attached branch, clean workspace, nothing in progress.
func (a *App) requireIdleBranch(ctx context.Context, dir, action string) (root, branch, head string, err error) {
	p, e := a.ResolveProject(ctx, dir)
	if e != nil {
		return "", "", "", e
	}
	root = p.RootPath
	if kind, e := a.gitRunner.OperationInProgress(ctx, root); e != nil {
		return "", "", "", e
	} else if kind != "" {
		return "", "", "", fmt.Errorf("a %s is in progress; continue or abort it first", kind)
	}
	if branch, e = a.gitRunner.CurrentBranch(ctx, root); e != nil || branch == "" {
		return "", "", "", fmt.Errorf("%s requires an attached local branch (detached HEAD is not accepted)", action)
	}
	changes, e := a.gitRunner.ReadChanges(ctx, root, branch)
	if e != nil {
		return "", "", "", e
	}
	if changes == nil || !changes.IsClean() {
		return "", "", "", fmt.Errorf("%s blocked: commit or discard your changes first; stage and files were not changed", action)
	}
	head, e = a.gitRunner.RevParseHead(ctx, root)
	return root, branch, head, e
}

func (a *App) PrepareCherryPick(ctx context.Context, dir, rev string) (*domain.CherryPickPlan, error) {
	root, branch, head, e := a.requireIdleBranch(ctx, dir, "cherry-pick")
	if e != nil {
		return nil, e
	}
	if strings.HasPrefix(strings.TrimSpace(rev), "-") || strings.TrimSpace(rev) == "" {
		return nil, fmt.Errorf("invalid commit")
	}
	commit, e := a.gitRunner.ResolveCommit(ctx, root, rev)
	if e != nil {
		return nil, fmt.Errorf("commit %q not found", rev)
	}
	subject, parents, e := a.gitRunner.CommitSummary(ctx, root, commit)
	if e != nil {
		return nil, e
	}
	if parents > 1 {
		return nil, fmt.Errorf("commit %s is a merge; cherry-pick of merges is not supported", commit[:min(12, len(commit))])
	}
	if n, e := a.gitRunner.CountRange(ctx, root, head, commit); e != nil {
		return nil, e
	} else if n == 0 || a.gitRunner.PatchApplied(ctx, root, head, commit) {
		return nil, fmt.Errorf("commit %s is already in %s", commit[:min(12, len(commit))], branch)
	}
	return &domain.CherryPickPlan{WorkspacePath: root, Branch: branch, HeadHash: head, Commit: commit, Subject: subject}, nil
}

func (a *App) CherryPick(ctx context.Context, dir string, reviewed domain.CherryPickPlan) (*domain.OperationResult, error) {
	plan, e := a.PrepareCherryPick(ctx, dir, reviewed.Commit)
	if e != nil {
		return nil, e
	}
	if *plan != reviewed {
		return nil, fmt.Errorf("branch or HEAD changed; review the cherry-pick again")
	}
	out, err := a.gitRunner.CherryPick(ctx, plan.WorkspacePath, plan.Commit)
	return a.operationOutcome(ctx, plan.WorkspacePath, "cherry-pick", out, err)
}

// PrepareRebase is read-only. Decision 2: only unpublished commits may be
// rebased, so vorq never needs a force push.
func (a *App) PrepareRebase(ctx context.Context, dir string) (*domain.RebasePlan, error) {
	root, branch, head, e := a.requireIdleBranch(ctx, dir, "rebase")
	if e != nil {
		return nil, e
	}
	r := a.gitRunner
	link, e := a.GetRepositoryLink(ctx, root)
	if e != nil {
		return nil, e
	}
	if link.Remote == "" {
		return nil, fmt.Errorf("no remote configured; rebase replays the branch onto the remote default branch")
	}
	plan := &domain.RebasePlan{WorkspacePath: root, Branch: branch, HeadHash: head, Remote: link.Remote}
	if plan.Base, e = a.syncBase(ctx, root, link.Remote); e != nil {
		return nil, e
	}
	if plan.Base == branch {
		return nil, fmt.Errorf("already on %s; rebase applies to feature branches (use the regular update)", branch)
	}
	tracking, e := r.RemoteTrackingHash(ctx, root, link.Remote, plan.Base)
	if e != nil || tracking == "" {
		return nil, fmt.Errorf("%s/%s is not known locally; use Trazer main or fetch first", link.Remote, plan.Base)
	}
	if plan.Commits, e = r.CountRange(ctx, root, tracking, head); e != nil {
		return nil, e
	}
	unpublished, e := r.UnpublishedCount(ctx, root, head)
	if e != nil {
		return nil, e
	}
	if published := plan.Commits - unpublished; published > 0 {
		return nil, fmt.Errorf("rebase blocked: %d commit(s) of %s are already published, and rewriting them would need a force push; use Trazer main (merge) instead", published, branch)
	}
	if plan.Behind, e = r.CountRange(ctx, root, head, tracking); e != nil {
		return nil, e
	}
	if plan.SourceID, e = r.PullSourceID(ctx, root, link.Remote); e != nil {
		return nil, fmt.Errorf("cannot identify the remote source")
	}
	return plan, nil
}

// Rebase fetches the base into a private ref, re-checks the review and that
// nothing became published, then replays the branch onto the pinned commit.
func (a *App) Rebase(ctx context.Context, dir string, reviewed domain.RebasePlan) (*domain.OperationResult, error) {
	check := func() (*domain.RebasePlan, error) {
		plan, e := a.PrepareRebase(ctx, dir)
		if e != nil {
			return nil, e
		}
		if plan.WorkspacePath != reviewed.WorkspacePath || plan.Branch != reviewed.Branch || plan.HeadHash != reviewed.HeadHash || plan.Base != reviewed.Base || plan.Remote != reviewed.Remote || plan.SourceID != reviewed.SourceID {
			return nil, fmt.Errorf("branch, base or HEAD changed; review the rebase again")
		}
		return plan, nil
	}
	plan, e := check()
	if e != nil {
		return nil, e
	}
	r := a.gitRunner
	netCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	hash, ref, e := r.FetchBranch(netCtx, plan.WorkspacePath, plan.Remote, plan.Base)
	if ref != "" {
		defer func() { _ = r.DropPullRef(context.WithoutCancel(ctx), plan.WorkspacePath, ref, hash) }()
	}
	if e != nil {
		return nil, fmt.Errorf("fetch not completed; HEAD preserved: %w", e)
	}
	if _, e = check(); e != nil {
		return nil, fmt.Errorf("fetch completed, rebase blocked: %w", e)
	}
	if n, e := r.CountRange(ctx, plan.WorkspacePath, plan.HeadHash, hash); e != nil {
		return nil, e
	} else if n == 0 {
		return &domain.OperationResult{Kind: "rebase", Output: fmt.Sprintf("%s already contains %s/%s; nothing to do", plan.Branch, plan.Remote, plan.Base)}, nil
	}
	out, err := r.Rebase(ctx, plan.WorkspacePath, hash)
	return a.operationOutcome(ctx, plan.WorkspacePath, "rebase", out, err)
}
