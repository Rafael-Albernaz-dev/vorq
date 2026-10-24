package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func (a *App) AuditWorkspaceSafety(ctx context.Context, currentDir string, ws *domain.Workspace) (*domain.SafetyReport, error) {
	if ws == nil {
		return nil, fmt.Errorf("workspace identity unavailable")
	}
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
	currentRoot, _, err := a.gitRunner.ProjectPaths(ctx, currentDir)
	if err != nil {
		return nil, fmt.Errorf("cannot establish active workspace: %w", err)
	}
	absCurrent, err := filepath.Abs(currentRoot)
	if err != nil {
		return nil, err
	}
	absWs, err := filepath.Abs(ws.Path)
	if err != nil {
		return nil, err
	}
	if ws.IsCurrent || absCurrent == absWs {
		violations = append(violations, domain.SafetyViolation{
			Type:        domain.ViolationActiveWorkspace,
			Description: "cannot remove active workspace currently in use; switch to another directory first ('cd ..')",
			ItemPath:    ws.Path,
		})
	}

	// Fatal identities do not need a destructive-path inspection.
	if len(violations) > 0 {
		return &domain.SafetyReport{IsSafe: false, Violations: violations}, nil
	}
	if ws.IsBare || ws.IsPrunable {
		return nil, fmt.Errorf("workspace unavailable for safety audit: %q", ws.Path)
	}
	// Refresh instead of trusting the discovery snapshot, including nil status.
	changes, err := a.gitRunner.ReadChanges(ctx, ws.Path, ws.Branch)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect workspace status: %w", err)
	}
	if changes == nil {
		return nil, fmt.Errorf("workspace status unavailable; removal blocked")
	}
	// 3. Inspect staged, unstaged and untracked changes.
	{
		if st := changes.StagedCount(); st > 0 {
			violations = append(violations, domain.SafetyViolation{
				Type:        domain.ViolationDirtyWorkingTree,
				Description: fmt.Sprintf("workspace has %d staged change(s) not committed", st),
				ItemPath:    ws.Path,
			})
		}
		if unst := changes.UnstagedCount(); unst > 0 {
			violations = append(violations, domain.SafetyViolation{
				Type:        domain.ViolationDirtyWorkingTree,
				Description: fmt.Sprintf("workspace has %d unstaged modification(s)", unst),
				ItemPath:    ws.Path,
			})
		}
		if untr := changes.UntrackedCount(); untr > 0 {
			violations = append(violations, domain.SafetyViolation{
				Type:        domain.ViolationUntrackedFiles,
				Description: fmt.Sprintf("workspace has %d untracked file(s) that would be permanently lost", untr),
				ItemPath:    ws.Path,
			})
		}
	}

	// 4. Inspect ignored files (like .env, certs, etc.)
	ignored, err := a.gitRunner.IgnoredFiles(ctx, ws.Path)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect ignored files; removal blocked: %w", err)
	}
	if len(ignored) > 0 {
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

func (a *App) RemoveWorkspace(ctx context.Context, currentDir string, targetIdentifier string, force bool) (*domain.SafetyReport, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}

	ws, err := a.findWorkspace(ctx, currentDir, targetIdentifier)
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

	// Native Git owns deletion. A replaced directory or dangling symlink must
	// never be removed by a second recursive filesystem cleanup.
	if _, err := os.Lstat(ws.Path); err == nil {
		return report, fmt.Errorf("git worktree remove completed but path %q still exists; residual data preserved", ws.Path)
	} else if !os.IsNotExist(err) {
		return report, fmt.Errorf("verify removed workspace %q: %w", ws.Path, err)
	}

	return report, nil
}
