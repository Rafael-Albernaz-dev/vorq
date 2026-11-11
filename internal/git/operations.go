package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// OperationInProgress reports the stopped Git operation of this worktree:
// "merge", "cherry-pick", "rebase", "revert" or "" (VORQ-020).
func (r *Repository) OperationInProgress(ctx context.Context, dir string) (string, error) {
	gitDir, err := r.Exec(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	for _, m := range []struct{ marker, kind string }{{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"}, {"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"}, {"MERGE_HEAD", "merge"}} {
		_, err := os.Lstat(filepath.Join(strings.TrimSpace(gitDir), m.marker))
		if err == nil {
			return m.kind, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", nil
}

// CherryPick applies one commit; -x records its origin in the message.
func (r *Repository) CherryPick(ctx context.Context, dir, commit string) (string, error) {
	return r.Exec(ctx, dir, "-c", "core.editor=true", "cherry-pick", "-x", "--no-edit", "--allow-empty-message", commit)
}

// Rebase replays the current branch onto a pinned commit, non-interactively.
func (r *Repository) Rebase(ctx context.Context, dir, onto string) (string, error) {
	return r.Exec(ctx, dir, "-c", "core.editor=true", "-c", "sequence.editor=true", "rebase", "--no-autostash", "--no-update-refs", "--empty=drop", onto)
}

// ContinueOperation resumes a stopped merge, cherry-pick or rebase after the
// user resolved and staged the conflicts. No editor is opened.
func (r *Repository) ContinueOperation(ctx context.Context, dir, kind string) (string, error) {
	return r.Exec(ctx, dir, "-c", "core.editor=true", kind, "--continue")
}

func (r *Repository) AbortOperation(ctx context.Context, dir, kind string) error {
	_, err := r.Exec(ctx, dir, kind, "--abort")
	return err
}

// CommitSummary returns a commit's subject and parent count.
func (r *Repository) CommitSummary(ctx context.Context, dir, hash string) (string, int, error) {
	raw, err := r.Exec(ctx, dir, "log", "-1", "--format=%P%x00%s", "--end-of-options", hash)
	if err != nil {
		return "", 0, err
	}
	parents, subject, _ := strings.Cut(strings.TrimSuffix(raw, "\n"), "\x00")
	return subject, len(strings.Fields(parents)), nil
}

// PatchApplied reports whether an equivalent patch of commit (same patch-id)
// is already in head, e.g. a previous cherry-pick of it.
func (r *Repository) PatchApplied(ctx context.Context, dir, head, commit string) bool {
	raw, err := r.Exec(ctx, dir, "cherry", "--end-of-options", head, commit, commit+"^")
	return err == nil && strings.HasPrefix(strings.TrimSpace(raw), "-")
}
