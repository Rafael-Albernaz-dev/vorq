package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type BranchDeleteRepository interface {
	UnpublishedCount(context.Context, string, string) (int, error)
	LocalBranchHash(context.Context, string, string) (string, error)
	DeleteRemoteBranch(context.Context, string, string, string, string) (string, error)
}
type BranchDeleteService interface {
	PrepareBranchDelete(context.Context, string, string) (*domain.BranchDeletePlan, error)
	DeleteBranch(context.Context, string, domain.BranchDeletePlan, bool) (*domain.BranchDeleteResult, error)
}

// PrepareBranchDelete is read-only and never fetches: merged/remote state is
// what the local refs know.
func (a *App) PrepareBranchDelete(ctx context.Context, dir, name string) (*domain.BranchDeletePlan, error) {
	r := a.gitRunner
	p, e := a.ResolveProject(ctx, dir)
	if e != nil {
		return nil, e
	}
	root := p.RootPath
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") {
		return nil, fmt.Errorf("invalid branch name")
	}
	plan := &domain.BranchDeletePlan{WorkspacePath: root, Branch: name}
	if plan.Hash, e = r.LocalBranchHash(ctx, root, name); e != nil {
		return nil, e
	}
	if plan.Hash == "" {
		return nil, fmt.Errorf("local branch %q not found", name)
	}
	branches, e := r.LocalBranches(ctx, root)
	if e != nil {
		return nil, e
	}
	for _, b := range branches {
		if b.Name != name {
			continue
		}
		if b.Current {
			return nil, fmt.Errorf("cannot delete the current branch %s; switch to another branch first", name)
		}
		if b.WorktreePath != "" {
			return nil, fmt.Errorf("branch %s is checked out in worktree %s; remove or switch that worktree first", name, b.WorktreePath)
		}
	}
	link, e := a.GetRepositoryLink(ctx, root)
	if e != nil {
		return nil, e
	}
	baseRefs := []string{}
	if link.Remote != "" {
		if base, err := a.syncBase(ctx, root, link.Remote); err == nil {
			plan.Base = base
			if h, _ := r.RemoteTrackingHash(ctx, root, link.Remote, base); h != "" {
				baseRefs = append(baseRefs, h)
			}
		}
	}
	if plan.Base == "" {
		if plan.Base, e = a.defaultBase(ctx, root); e != nil {
			return nil, e
		}
	}
	if plan.Base == name {
		return nil, fmt.Errorf("cannot delete %s: it is the repository's default branch", name)
	}
	if h, _ := r.LocalBranchHash(ctx, root, plan.Base); h != "" {
		baseRefs = append(baseRefs, h)
	}
	merged := func(hash string) bool {
		for _, base := range baseRefs {
			if n, err := r.CountRange(ctx, root, base, hash); err == nil && n == 0 {
				return true
			}
		}
		return false
	}
	plan.Merged = merged(plan.Hash)
	if plan.Unpublished, e = r.UnpublishedCount(ctx, root, plan.Hash); e != nil {
		return nil, e
	}
	remote, ref, _ := r.UpstreamTarget(ctx, root, name)
	remoteBranch := strings.TrimPrefix(ref, "refs/heads/")
	if remote == "" || remote == "." || remoteBranch == "" {
		remote, remoteBranch = link.Remote, name
	}
	if remote != "" && remoteBranch == name {
		if h, _ := r.RemoteTrackingHash(ctx, root, remote, name); h != "" {
			plan.Remote, plan.RemoteHash, plan.RemoteMerged = remote, h, merged(h)
		}
	}
	return plan, nil
}

// DeleteBranch deletes the remote branch first (pinned by lease), then the
// local one pinned to the reviewed hash. Losing commits needs confirmed=true.
func (a *App) DeleteBranch(ctx context.Context, dir string, reviewed domain.BranchDeletePlan, confirmed bool) (*domain.BranchDeleteResult, error) {
	plan, e := a.PrepareBranchDelete(ctx, dir, reviewed.Branch)
	if e != nil {
		return nil, e
	}
	if *plan != reviewed {
		return nil, fmt.Errorf("branch, remote or base changed; review the deletion again")
	}
	if plan.NeedsConfirm() && !confirmed {
		return nil, fmt.Errorf("branch %s has commits not merged into %s; deleting needs the typed confirmation %s", plan.Branch, plan.Base, domain.GuardOverrideWord)
	}
	r := a.gitRunner
	result := &domain.BranchDeleteResult{Plan: *plan}
	if plan.RemoteHash != "" {
		netCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		_, e = r.DeleteRemoteBranch(netCtx, plan.WorkspacePath, plan.Remote, plan.Branch, plan.RemoteHash)
		cancel()
		if e != nil {
			return nil, fmt.Errorf("remote branch %s/%s not deleted (it may have new commits); local branch kept: %w", plan.Remote, plan.Branch, e)
		}
		result.RemoteDeleted = true
	}
	if h, err := r.LocalBranchHash(ctx, plan.WorkspacePath, plan.Branch); err != nil || h != plan.Hash {
		return result, fmt.Errorf("local branch %s changed after review; it was kept (remote deleted: %t)", plan.Branch, result.RemoteDeleted)
	}
	if e = r.BranchDelete(ctx, plan.WorkspacePath, plan.Branch, true); e != nil {
		return result, fmt.Errorf("local branch %s not deleted (remote deleted: %t): %w", plan.Branch, result.RemoteDeleted, e)
	}
	return result, nil
}
