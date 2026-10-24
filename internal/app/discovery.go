package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func (a *App) ListWorktrees(ctx context.Context, currentDir string) ([]*domain.Workspace, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	worktrees, err := a.gitRunner.ReadWorktrees(ctx, proj.RootPath)
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	absCurrent, err := filepath.Abs(proj.RootPath)
	if err != nil {
		return nil, err
	}
	jobs := make(chan int, len(worktrees))
	for i, ws := range worktrees {
		if ws == nil {
			return nil, fmt.Errorf("invalid empty worktree entry")
		}
		abs, err := filepath.Abs(ws.Path)
		if err != nil {
			return nil, err
		}
		ws.IsCurrent = abs == absCurrent
		if !ws.IsBare && !ws.IsPrunable {
			jobs <- i
		}
	}
	close(jobs)
	failures := make([]error, len(worktrees))
	var wg sync.WaitGroup
	for worker := 0; worker < min(4, len(worktrees)); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				ws := worktrees[index]
				if err := ctx.Err(); err != nil {
					failures[index] = err
					continue
				}
				changes, err := a.gitRunner.ReadChanges(ctx, ws.Path, ws.Branch)
				if err == nil && changes == nil {
					err = fmt.Errorf("status unavailable")
				}
				if err == nil {
					changes.Ahead, changes.Behind, err = a.gitRunner.AheadBehind(ctx, ws.Path)
				}
				if err != nil {
					failures[index] = fmt.Errorf("inspect worktree %q: %w", ws.Path, err)
					continue
				}
				ws.Status = changes
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := errors.Join(failures...); err != nil {
		return nil, err
	}
	return worktrees, nil
}

func (a *App) findWorkspace(ctx context.Context, currentDir string, targetIdentifier string) (*domain.Workspace, error) {
	worktrees, err := a.ListWorktrees(ctx, currentDir)
	if err != nil {
		return nil, err
	}

	var target *domain.Workspace
	if targetIdentifier == "" {
		// Use current workspace
		for _, ws := range worktrees {
			if ws.IsCurrent {
				target = ws
				break
			}
		}
	} else {
		// Prefer exact paths, then branch names. Abbreviated paths must be unique.
		for _, ws := range worktrees {
			if ws.Path == targetIdentifier {
				target = ws
				break
			}
		}
		if target == nil {
			for _, ws := range worktrees {
				if ws.Branch == targetIdentifier {
					target = ws
					break
				}
			}
		}
		if target == nil {
			for _, ws := range worktrees {
				if strings.HasSuffix(ws.Path, "/"+targetIdentifier) || strings.HasSuffix(ws.Path, "\\"+targetIdentifier) {
					if target != nil {
						return nil, fmt.Errorf("workspace %q is ambiguous; use its full path or branch", targetIdentifier)
					}
					target = ws
				}
			}
		}
	}

	if target == nil {
		return nil, fmt.Errorf("workspace %q not found", targetIdentifier)
	}

	if target.IsBare || target.IsPrunable || target.Status == nil {
		return nil, fmt.Errorf("workspace %q is unavailable for inspection", target.Path)
	}
	return target, nil
}

func (a *App) GetWorkspaceStatus(ctx context.Context, currentDir, targetIdentifier string) (*domain.Workspace, *domain.ChangeSet, error) {
	target, err := a.findWorkspace(ctx, currentDir, targetIdentifier)
	if err != nil {
		return nil, nil, err
	}

	stagedDiff, err := a.gitRunner.DiffCached(ctx, target.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read staged diff: %w", err)
	}
	unstagedDiff, err := a.gitRunner.Diff(ctx, target.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("read working diff: %w", err)
	}

	if target.Status != nil {
		target.Status.StagedDiff = stagedDiff
		target.Status.UnstagedDiff = unstagedDiff
	}

	return target, target.Status, nil
}
