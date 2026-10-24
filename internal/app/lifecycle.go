package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/files"
)

func (a *App) SpawnWorkspace(ctx context.Context, currentDir string, opts SpawnOptions) (*SpawnResult, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(opts.BranchName) == "" {
		return nil, fmt.Errorf("branch name is required")
	}

	if opts.BaseBranch == "" {
		opts.BaseBranch = proj.DefaultBaseBranch
	}

	// 1. Pre-flight check on branch
	branchExists, err := a.gitRunner.BranchExists(ctx, proj.RootPath, opts.BranchName)
	if err != nil {
		return nil, fmt.Errorf("failed to check branch existence: %w", err)
	}

	if branchExists && !opts.CheckoutExisting {
		return nil, fmt.Errorf("branch %q already exists. Use --checkout to attach to an existing branch", opts.BranchName)
	}
	if !branchExists && opts.CheckoutExisting {
		return nil, fmt.Errorf("cannot checkout: branch %q does not exist", opts.BranchName)
	}

	// 2. Resolve destination path
	targetDir := opts.TargetDir
	if targetDir == "" {
		repoName := filepath.Base(proj.RootPath)
		cleanBranch := strings.ReplaceAll(opts.BranchName, "/", "-")
		cleanBranch = strings.ReplaceAll(cleanBranch, "\\", "-")
		targetDir = filepath.Join(filepath.Dir(proj.RootPath), repoName+"-worktrees", cleanBranch)
	}

	absTargetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("invalid destination directory path: %w", err)
	}

	// 3. Pre-flight check on destination directory
	if info, err := os.Stat(absTargetDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(absTargetDir)
		if len(entries) > 0 {
			return nil, fmt.Errorf("destination directory %q already exists and is not empty", absTargetDir)
		}
	}

	// 4. Provision Git worktree
	if err := a.gitRunner.WorktreeAdd(ctx, proj.RootPath, opts.BranchName, absTargetDir, opts.BaseBranch, opts.CheckoutExisting); err != nil {
		return nil, fmt.Errorf("failed to create git worktree: %w", err)
	}

	// 5. Copy declared local files with rollback protection
	copiedFiles, err := files.SyncLocalFiles(proj.RootPath, absTargetDir, opts.CopyFiles)
	if err != nil {
		// ROLLBACK: Remove the worktree that was just created
		_ = a.gitRunner.WorktreeRemove(ctx, proj.RootPath, absTargetDir, true)
		if !opts.CheckoutExisting {
			_ = a.gitRunner.BranchDelete(ctx, proj.RootPath, opts.BranchName, true)
		}
		return nil, fmt.Errorf("failed during file synchronization (rolled back worktree): %w", err)
	}

	return &SpawnResult{
		Workspace: &domain.Workspace{
			Path:      absTargetDir,
			Branch:    opts.BranchName,
			IsMain:    false,
			IsCurrent: false,
			CreatedAt: time.Now(),
		},
		CopiedFiles:    copiedFiles,
		IsReusedBranch: opts.CheckoutExisting,
	}, nil
}

func (a *App) PruneWorkspaces(ctx context.Context, currentDir string) (string, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return "", err
	}
	return a.gitRunner.WorktreePrune(ctx, proj.RootPath)
}
