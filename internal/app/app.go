package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/files"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

// Service defines the core use case contracts available to both CLI and TUI.
type Service interface {
	GetGitVersion(ctx context.Context) (string, error)
	ResolveProject(ctx context.Context, dir string) (*domain.Project, error)
	ListWorktrees(ctx context.Context, currentDir string) ([]*domain.Workspace, error)
	GetWorkspaceStatus(ctx context.Context, currentDir string, targetIdentifier string) (*domain.Workspace, *domain.ChangeSet, error)
	SpawnWorkspace(ctx context.Context, currentDir string, opts SpawnOptions) (*SpawnResult, error)
	AuditWorkspaceSafety(ctx context.Context, currentDir string, ws *domain.Workspace) (*domain.SafetyReport, error)
	RemoveWorkspace(ctx context.Context, currentDir string, targetIdentifier string, force bool) (*domain.SafetyReport, error)
	PruneWorkspaces(ctx context.Context, currentDir string) (string, error)
	GetCommitChanges(ctx context.Context, currentDir string) (*domain.ChangeSet, error)
	StageFiles(ctx context.Context, currentDir string, paths []string) error
	UnstageFiles(ctx context.Context, currentDir string, paths []string) error
	ProposeCommit(ctx context.Context, currentDir string, commitType string) (*domain.CommitProposal, error)
	PreviewCommit(ctx context.Context, currentDir, commitType string, all bool) (*domain.ChangeSet, *domain.CommitProposal, error)
	CreateCommit(ctx context.Context, currentDir string, message string) (*domain.CommitResult, error)
	CreateReviewedCommit(ctx context.Context, currentDir, message, expectedDiff string) (*domain.CommitResult, error)
	GetPushTarget(ctx context.Context, currentDir string) (*domain.PushTarget, error)
	PushChanges(ctx context.Context, currentDir string, opts domain.PushOptions) (*domain.PushResult, error)
	GetWorkspaceTree(ctx context.Context, currentDir string) (*domain.WorkspaceTree, error)
	GetFileContent(ctx context.Context, currentDir, path string) (*domain.FileContent, error)
	GetFileDiff(ctx context.Context, currentDir, path string) (*domain.FileDiff, error)
	ToggleStage(ctx context.Context, currentDir, path string) error
}

// SpawnOptions holds parameters for provisioning a new worktree.
type SpawnOptions struct {
	BranchName       string
	BaseBranch       string
	TargetDir        string
	CheckoutExisting bool
	CopyFiles        []string
}

// SpawnResult holds the outcome of a spawn operation.
type SpawnResult struct {
	Workspace      *domain.Workspace
	CopiedFiles    []string
	IsReusedBranch bool
}

// App implements the Service interface coordinating adapters and business logic.
type App struct {
	gitRunner git.Runner
}

// NewApp initializes the application core with required adapters.
func NewApp(gitRunner git.Runner) *App {
	return &App{
		gitRunner: gitRunner,
	}
}

// GetGitVersion queries the host git binary version.
func (a *App) GetGitVersion(ctx context.Context) (string, error) {
	return a.gitRunner.Version(ctx)
}

// ResolveProject inspects the given directory and discovers the project Git root and common dir.
func (a *App) ResolveProject(ctx context.Context, dir string) (*domain.Project, error) {
	commonDir, err := a.gitRunner.Exec(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("not a git repository (or any parent up to mount point): %w", err)
	}

	rootDir, err := a.gitRunner.Exec(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("unable to resolve top-level git repository: %w", err)
	}

	commonDir = strings.TrimSpace(commonDir)
	rootDir = strings.TrimSpace(rootDir)

	return &domain.Project{
		ID:                commonDir,
		RootPath:          rootDir,
		GitDir:            commonDir,
		DefaultBaseBranch: "main",
	}, nil
}

// ListWorktrees discovers all worktrees and concurrently inspects their status.
func (a *App) ListWorktrees(ctx context.Context, currentDir string) ([]*domain.Workspace, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}

	raw, err := a.gitRunner.ListWorktrees(ctx, proj.RootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	worktrees, err := git.ParseWorktreeList(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse worktrees: %w", err)
	}

	// Resolve current absolute path
	absCurrentDir, _ := filepath.Abs(currentDir)

	// Concurrently query status for each worktree (worker pool / sync.WaitGroup)
	var wg sync.WaitGroup
	for _, ws := range worktrees {
		absWsPath, _ := filepath.Abs(ws.Path)
		if absWsPath == absCurrentDir {
			ws.IsCurrent = true
		}

		wg.Add(1)
		go func(w *domain.Workspace) {
			defer wg.Done()
			statusRaw, err := a.gitRunner.Status(ctx, w.Path)
			if err != nil {
				return
			}
			cs, err := git.ParseStatus(statusRaw, w.Path, w.Branch)
			if err != nil {
				return
			}
			ahead, behind, _ := a.gitRunner.AheadBehind(ctx, w.Path)
			cs.Ahead = ahead
			cs.Behind = behind
			w.Status = cs
		}(ws)
	}
	wg.Wait()

	return worktrees, nil
}

// GetWorkspaceStatus fetches detailed changes and diffs for a specific workspace.
func (a *App) GetWorkspaceStatus(ctx context.Context, currentDir string, targetIdentifier string) (*domain.Workspace, *domain.ChangeSet, error) {
	worktrees, err := a.ListWorktrees(ctx, currentDir)
	if err != nil {
		return nil, nil, err
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
		if target == nil && len(worktrees) > 0 {
			target = worktrees[0]
		}
	} else {
		// Match by branch name or path suffix
		for _, ws := range worktrees {
			if ws.Branch == targetIdentifier ||
				ws.Path == targetIdentifier ||
				strings.HasSuffix(ws.Path, "/"+targetIdentifier) ||
				strings.HasSuffix(ws.Path, "\\"+targetIdentifier) {
				target = ws
				break
			}
		}
	}

	if target == nil {
		return nil, nil, fmt.Errorf("workspace %q not found", targetIdentifier)
	}

	// Fetch staged and unstaged diffs for target
	stagedDiff, _ := a.gitRunner.DiffCached(ctx, target.Path)
	unstagedDiff, _ := a.gitRunner.Diff(ctx, target.Path)

	if target.Status != nil {
		target.Status.StagedDiff = stagedDiff
		target.Status.UnstagedDiff = unstagedDiff
	}

	return target, target.Status, nil
}

// SpawnWorkspace provisions a new isolated worktree, creating the branch and copying declared files.
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

// AuditWorkspaceSafety performs a multi-vector risk audit on a workspace before deletion.
func (a *App) AuditWorkspaceSafety(ctx context.Context, currentDir string, ws *domain.Workspace) (*domain.SafetyReport, error) {
	var violations []domain.SafetyViolation

	// 1. Guard against removing the main repository working tree
	if ws.IsMain {
		violations = append(violations, domain.SafetyViolation{
			Type:        domain.ViolationMainWorkspace,
			Description: "cannot remove main working tree; only secondary worktrees can be removed",
			ItemPath:    ws.Path,
		})
	}

	// 2. Guard against removing the current active directory
	absCurrent, _ := filepath.Abs(currentDir)
	absWs, _ := filepath.Abs(ws.Path)
	if ws.IsCurrent || absCurrent == absWs {
		violations = append(violations, domain.SafetyViolation{
			Type:        domain.ViolationActiveWorkspace,
			Description: "cannot remove active workspace currently in use; switch to another directory first ('cd ..')",
			ItemPath:    ws.Path,
		})
	}

	// 3. Inspect staged, unstaged and untracked changes
	if ws.Status != nil {
		if st := ws.Status.StagedCount(); st > 0 {
			violations = append(violations, domain.SafetyViolation{
				Type:        domain.ViolationDirtyWorkingTree,
				Description: fmt.Sprintf("workspace has %d staged change(s) not committed", st),
				ItemPath:    ws.Path,
			})
		}
		if unst := ws.Status.UnstagedCount(); unst > 0 {
			violations = append(violations, domain.SafetyViolation{
				Type:        domain.ViolationDirtyWorkingTree,
				Description: fmt.Sprintf("workspace has %d unstaged modification(s)", unst),
				ItemPath:    ws.Path,
			})
		}
		if untr := ws.Status.UntrackedCount(); untr > 0 {
			violations = append(violations, domain.SafetyViolation{
				Type:        domain.ViolationUntrackedFiles,
				Description: fmt.Sprintf("workspace has %d untracked file(s) that would be permanently lost", untr),
				ItemPath:    ws.Path,
			})
		}
	}

	// 4. Inspect ignored files (like .env, certs, etc.)
	ignored, err := a.gitRunner.IgnoredFiles(ctx, ws.Path)
	if err == nil && len(ignored) > 0 {
		previewCount := len(ignored)
		if previewCount > 3 {
			previewCount = 3
		}
		preview := strings.Join(ignored[:previewCount], ", ")
		violations = append(violations, domain.SafetyViolation{
			Type:        domain.ViolationIgnoredFiles,
			Description: fmt.Sprintf("workspace contains %d ignored file(s) that would be permanently deleted (%s)", len(ignored), preview),
			ItemPath:    ws.Path,
		})
	}

	return &domain.SafetyReport{
		IsSafe:     len(violations) == 0,
		Violations: violations,
	}, nil
}

// RemoveWorkspace safely removes a worktree after auditing safety constraints.
func (a *App) RemoveWorkspace(ctx context.Context, currentDir string, targetIdentifier string, force bool) (*domain.SafetyReport, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}

	ws, _, err := a.GetWorkspaceStatus(ctx, currentDir, targetIdentifier)
	if err != nil {
		return nil, err
	}

	report, err := a.AuditWorkspaceSafety(ctx, currentDir, ws)
	if err != nil {
		return nil, fmt.Errorf("safety audit failed: %w", err)
	}

	// Check fatal invariants that can NEVER be overridden even with --force
	for _, v := range report.Violations {
		if v.Type == domain.ViolationMainWorkspace || v.Type == domain.ViolationActiveWorkspace {
			return report, fmt.Errorf("blocked: %s", v.Description)
		}
	}

	// If there are other violations (dirty, untracked, ignored) and force is false, block
	if !report.IsSafe && !force {
		return report, fmt.Errorf("safety check blocked removal: %d violation(s) found", len(report.Violations))
	}

	// Execute Git worktree remove
	if err := a.gitRunner.WorktreeRemove(ctx, proj.RootPath, ws.Path, force); err != nil {
		return report, fmt.Errorf("failed to remove git worktree: %w", err)
	}

	// If directory still exists on disk (due to untracked/ignored residues), clean it up
	if _, err := os.Stat(ws.Path); err == nil {
		_ = os.RemoveAll(ws.Path)
	}

	return report, nil
}

// PruneWorkspaces executes git worktree prune.
func (a *App) PruneWorkspaces(ctx context.Context, currentDir string) (string, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return "", err
	}
	return a.gitRunner.WorktreePrune(ctx, proj.RootPath)
}
