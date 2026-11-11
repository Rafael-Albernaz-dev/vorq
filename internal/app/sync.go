package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// SyncRepository brings the remote default branch into the current branch
// with a pinned fetch and a native merge (VORQ-019). History is never rewritten.
type SyncRepository interface {
	FetchBranch(context.Context, string, string, string) (string, string, error)
	RemoteTrackingHash(context.Context, string, string, string) (string, error)
	CountRange(context.Context, string, string, string) (int, error)
	MergeInProgress(context.Context, string) (bool, error)
	MergeBranch(context.Context, string, string, string) (string, error)
	UnmergedPaths(context.Context, string) ([]string, error)
	MergeAbort(context.Context, string) error
}
type SyncService interface {
	GetSyncTarget(context.Context, string) (*domain.SyncTarget, error)
	SyncFromBase(context.Context, string, domain.SyncTarget) (*domain.SyncResult, error)
	AbortMerge(context.Context, string) error
}

// GetSyncTarget is read-only: it never fetches. Behind counts only what the
// local remote-tracking ref already knows.
func (a *App) GetSyncTarget(ctx context.Context, dir string) (*domain.SyncTarget, error) {
	r := a.gitRunner
	p, e := a.ResolveProject(ctx, dir)
	if e != nil {
		return nil, e
	}
	root := p.RootPath
	if kind, e := r.OperationInProgress(ctx, root); e != nil {
		return nil, e
	} else if kind != "" && kind != "merge" {
		return nil, fmt.Errorf("a %s is in progress; continue or abort it first (Git → Operação em andamento, or vorq continue / vorq abort)", kind)
	}
	branch, e := r.CurrentBranch(ctx, root)
	if e != nil || branch == "" {
		return nil, fmt.Errorf("updating from the base requires an attached local branch (detached HEAD is not accepted)")
	}
	t := &domain.SyncTarget{WorkspacePath: root, Branch: branch}
	if t.MergeInProgress, e = r.MergeInProgress(ctx, root); e != nil {
		return nil, e
	}
	if t.MergeInProgress {
		if t.Conflicts, e = r.UnmergedPaths(ctx, root); e != nil {
			return nil, e
		}
		return t, nil
	}
	if t.HeadHash, e = r.RevParseHead(ctx, root); e != nil {
		return nil, e
	}
	link, e := a.GetRepositoryLink(ctx, root)
	if e != nil {
		return nil, e
	}
	if link.Remote == "" {
		return nil, fmt.Errorf("no remote configured; add one before updating from the base branch")
	}
	t.Remote = link.Remote
	if t.Base, e = a.syncBase(ctx, root, t.Remote); e != nil {
		return nil, e
	}
	if t.OnBase = branch == t.Base; t.OnBase {
		return t, nil
	}
	changes, e := r.ReadChanges(ctx, root, branch)
	if e != nil {
		return nil, e
	}
	if changes == nil || !changes.IsClean() {
		return nil, fmt.Errorf("update blocked: commit or discard your changes before bringing %s/%s; stage and files were not changed", t.Remote, t.Base)
	}
	if t.SourceID, e = r.PullSourceID(ctx, root, t.Remote); e != nil {
		return nil, fmt.Errorf("cannot identify the remote source")
	}
	if tracking, _ := r.RemoteTrackingHash(ctx, root, t.Remote, t.Base); tracking != "" {
		if t.Behind, e = r.CountRange(ctx, root, t.HeadHash, tracking); e != nil {
			return nil, e
		}
	}
	return t, nil
}

// syncBase is the remote default branch (refs/remotes/<remote>/HEAD), else a
// known main/master remote-tracking ref. It never fetches.
func (a *App) syncBase(ctx context.Context, root, remote string) (string, error) {
	if name, e := a.gitRunner.RemoteDefaultBranch(ctx, root, remote); e == nil && name != "" {
		return name, nil
	}
	for _, name := range []string{"main", "master"} {
		if hash, e := a.gitRunner.RemoteTrackingHash(ctx, root, remote, name); e == nil && hash != "" {
			return name, nil
		}
	}
	return "", fmt.Errorf("cannot identify the default branch of %s; run: git remote set-head %s --auto", remote, remote)
}

func sameSyncTarget(a, b domain.SyncTarget) bool {
	return a.WorkspacePath == b.WorkspacePath && a.Branch == b.Branch && a.Remote == b.Remote && a.Base == b.Base && a.HeadHash == b.HeadHash && a.SourceID == b.SourceID
}

func (a *App) syncReady(ctx context.Context, dir string, reviewed domain.SyncTarget) (*domain.SyncTarget, error) {
	t, e := a.GetSyncTarget(ctx, dir)
	if e != nil {
		return nil, e
	}
	switch {
	case t.MergeInProgress:
		return nil, fmt.Errorf("a merge is already in progress; resolve and commit it, or abort it")
	case t.OnBase:
		return nil, fmt.Errorf("already on %s; use the regular update (pull)", t.Base)
	case !sameSyncTarget(*t, reviewed):
		return nil, fmt.Errorf("base, branch or HEAD changed; review the update again")
	}
	return t, nil
}

// SyncFromBase fetches the reviewed base and merges it. Conflicts are not an
// error: the merge stops for resolution and the result lists the paths.
func (a *App) SyncFromBase(ctx context.Context, dir string, reviewed domain.SyncTarget) (result *domain.SyncResult, err error) {
	target, e := a.syncReady(ctx, dir, reviewed)
	if e != nil {
		return nil, e
	}
	r := a.gitRunner
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	hash, ref, e := r.FetchBranch(ctx, target.WorkspacePath, target.Remote, target.Base)
	if ref != "" {
		defer func() {
			cleanCtx, cleanCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cleanCancel()
			if cleanErr := r.DropPullRef(cleanCtx, target.WorkspacePath, ref, hash); cleanErr != nil {
				if result != nil {
					result.Output += "\nAviso: referência temporária da atualização foi preservada."
				} else {
					err = errors.Join(err, fmt.Errorf("temporary ref cleanup: %w", cleanErr))
				}
			}
		}()
	}
	if e != nil {
		return nil, fmt.Errorf("fetch not completed; HEAD preserved, remote-tracking refs may have been updated: %w", e)
	}
	if _, e = a.syncReady(ctx, dir, reviewed); e != nil {
		return nil, fmt.Errorf("fetch completed, merge blocked: %w", e)
	}
	result = &domain.SyncResult{Target: *target}
	if result.Incoming, e = r.CountRange(ctx, target.WorkspacePath, target.HeadHash, hash); e != nil {
		return nil, e
	}
	if result.Incoming == 0 {
		result.UpToDate = true
		return result, nil
	}
	message := fmt.Sprintf("Merge remote-tracking branch '%s/%s' into %s", target.Remote, target.Base, target.Branch)
	out, mergeErr := r.MergeBranch(ctx, target.WorkspacePath, hash, message)
	result.Output = out
	if mergeErr == nil {
		return result, nil
	}
	stopped, _ := r.MergeInProgress(ctx, target.WorkspacePath)
	if !stopped {
		return nil, fmt.Errorf("merge not applied; HEAD and files preserved: %w", mergeErr)
	}
	result.Stopped = true
	result.Output = mergeErr.Error()
	if result.Conflicts, e = r.UnmergedPaths(ctx, target.WorkspacePath); e != nil {
		return nil, e
	}
	return result, nil
}

// AbortMerge restores the state before a stopped merge. It refuses when no
// merge is in progress, so it can never discard unrelated work.
func (a *App) AbortMerge(ctx context.Context, dir string) error {
	p, e := a.ResolveProject(ctx, dir)
	if e != nil {
		return e
	}
	inProgress, e := a.gitRunner.MergeInProgress(ctx, p.RootPath)
	if e != nil {
		return e
	}
	if !inProgress {
		return fmt.Errorf("no merge in progress; nothing was changed")
	}
	return a.gitRunner.MergeAbort(ctx, p.RootPath)
}
