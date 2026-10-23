package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// GetPushTarget only inspects local configuration and refs. It never fetches.
func (a *App) GetPushTarget(ctx context.Context, currentDir string) (*domain.PushTarget, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	branch, err := a.gitRunner.CurrentBranch(ctx, proj.RootPath)
	if err != nil {
		return nil, fmt.Errorf("push requires an attached local branch; switch from detached HEAD first")
	}
	head, err := a.gitRunner.RevParseHead(ctx, proj.RootPath)
	if err != nil {
		return nil, fmt.Errorf("create a local commit before publishing: %w", err)
	}
	remotes, err := a.gitRunner.Remotes(ctx, proj.RootPath)
	if err != nil {
		return nil, err
	}
	// Upstream atoms preserve remote names and destination branches containing /.
	upstream, err := a.gitRunner.Exec(ctx, proj.RootPath, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err != nil {
		return nil, err
	}
	remote, remoteRef, _ := strings.Cut(strings.TrimSuffix(upstream, "\n"), "\x00")
	target := &domain.PushTarget{WorkspacePath: proj.RootPath, Branch: branch, HeadHash: head, Ahead: -1}
	if remote != "" && strings.HasPrefix(remoteRef, "refs/heads/") {
		target.Remote, target.RemoteBranch, target.HasUpstream = remote, strings.TrimPrefix(remoteRef, "refs/heads/"), true
		if _, err := a.gitRunner.Upstream(ctx, proj.RootPath); err == nil {
			target.Ahead, target.Behind, err = a.gitRunner.AheadBehind(ctx, proj.RootPath)
			if err != nil {
				return nil, err
			}
		}
	} else {
		target.RemoteBranch = branch
		for _, name := range remotes {
			if name == "origin" {
				target.Remote = name
			}
		}
		if target.Remote == "" && len(remotes) == 1 {
			target.Remote = remotes[0]
		}
		if target.Remote == "" {
			if len(remotes) == 0 {
				return nil, fmt.Errorf("no remote configured; add a remote before publishing")
			}
			return nil, fmt.Errorf("multiple remotes and no origin/upstream; configure the branch upstream explicitly before publishing")
		}
	}
	urls, err := a.gitRunner.Exec(ctx, proj.RootPath, "remote", "get-url", "--push", "--all", target.Remote)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve push destination for remote %q", target.Remote)
	}
	target.DestinationID = fmt.Sprintf("%x", sha256.Sum256([]byte(urls)))
	return target, nil
}

// PushChanges rejects a changed preview. Confirmation belongs to the interface;
// the core still requires the exact reviewed branch, HEAD and destination.
func (a *App) PushChanges(ctx context.Context, currentDir string, opts domain.PushOptions) (*domain.PushResult, error) {
	target, err := a.GetPushTarget(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	expected := opts.Target
	if expected.WorkspacePath != target.WorkspacePath || expected.Branch != target.Branch || expected.Remote != target.Remote || expected.RemoteBranch != target.RemoteBranch || expected.HasUpstream != target.HasUpstream || expected.HeadHash != target.HeadHash || expected.DestinationID != target.DestinationID {
		return nil, fmt.Errorf("push target changed or was not reviewed; inspect the destination and confirm again")
	}
	out, err := a.gitRunner.Push(ctx, target.WorkspacePath, target.Remote, target.Branch, target.RemoteBranch, !target.HasUpstream)
	if err != nil {
		return nil, err
	}
	return &domain.PushResult{Target: *target, Output: out}, nil
}
