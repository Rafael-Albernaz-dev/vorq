package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// Repository adapts native command operations into the typed application port.
// Runner stays injectable so native failures can be exercised in isolation.
type Repository struct{ Runner }

func NewRepository(runner Runner) *Repository { return &Repository{Runner: runner} }

func (r *Repository) ProjectPaths(ctx context.Context, dir string) (string, string, error) {
	common, err := r.Exec(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", "", err
	}
	root, err := r.Exec(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	return strings.TrimSuffix(root, "\n"), strings.TrimSuffix(common, "\n"), nil
}
func (r *Repository) Branch(ctx context.Context, dir string) (string, bool, error) {
	branch, err := r.CurrentBranch(ctx, dir)
	if err == nil {
		return branch, false, nil
	}
	var exit *exec.ExitError
	if ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() == 1 {
		if _, headErr := r.RevParseHead(ctx, dir); headErr == nil {
			return "(detached HEAD)", true, nil
		}
	}
	return "", false, err
}
func (r *Repository) ReadWorktrees(ctx context.Context, dir string) ([]*domain.Workspace, error) {
	raw, err := r.ListWorktrees(ctx, dir)
	if err != nil {
		return nil, err
	}
	if raw != "" && (!strings.HasPrefix(raw, "worktree ") || !strings.HasSuffix(raw, "\x00")) {
		return nil, fmt.Errorf("invalid or truncated worktree list")
	}
	return ParseWorktreeList(raw)
}
func (r *Repository) ReadChanges(ctx context.Context, dir, branch string) (*domain.ChangeSet, error) {
	raw, err := r.Exec(ctx, dir, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--renames")
	if err != nil {
		return nil, err
	}
	return ParseStatus(raw, dir, branch)
}
func nulPaths(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
}
func (r *Repository) TrackedPaths(ctx context.Context, dir string) ([]string, error) {
	raw, err := r.Exec(ctx, dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	return nulPaths(raw), nil
}
func (r *Repository) IgnoredPath(ctx context.Context, dir, path string) (bool, error) {
	raw, err := r.Exec(ctx, dir, "--literal-pathspecs", "ls-files", "--cached", "--others", "--ignored", "--exclude-standard", "-z", "--", path)
	return raw != "", err
}
func (r *Repository) IndexEntry(ctx context.Context, dir, path string) (*domain.GitEntry, error) {
	raw, err := r.Exec(ctx, dir, "--literal-pathspecs", "ls-files", "--stage", "-z", "--", path)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	metadata, _, ok := strings.Cut(raw, "\t")
	fields := strings.Fields(metadata)
	if !ok || len(fields) != 3 || fields[2] != "0" || len(nulPaths(raw)) != 1 {
		return nil, fmt.Errorf("invalid or unmerged index entry for %q", path)
	}
	return &domain.GitEntry{Mode: fields[0], ObjectID: fields[1]}, nil
}
func (r *Repository) TreeEntry(ctx context.Context, dir, base, path string) (*domain.GitEntry, error) {
	raw, err := r.Exec(ctx, dir, "--literal-pathspecs", "ls-tree", "--full-tree", "-z", base, "--", path)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	metadata, _, ok := strings.Cut(raw, "\t")
	fields := strings.Fields(metadata)
	if !ok || len(fields) != 3 || len(nulPaths(raw)) != 1 {
		return nil, fmt.Errorf("invalid tree entry for %q", path)
	}
	return &domain.GitEntry{Mode: fields[0], ObjectID: fields[2]}, nil
}
func (r *Repository) RemoveIndexEntry(ctx context.Context, dir, path string) error {
	_, err := r.Exec(ctx, dir, "--literal-pathspecs", "rm", "--cached", "-f", "--", path)
	return err
}
func (r *Repository) RestoreFile(ctx context.Context, dir, path string, staged bool) error {
	args := []string{"--literal-pathspecs", "restore", "--worktree"}
	if staged {
		args = append(args, "--source=HEAD", "--staged")
	}
	_, err := r.Exec(ctx, dir, append(args, "--", path)...)
	return err
}
func (r *Repository) PreviewBase(ctx context.Context, dir string) (string, error) {
	if _, headErr := r.RevParseHead(ctx, dir); headErr != nil {
		state, err := r.Exec(ctx, dir, "--no-optional-locks", "status", "--porcelain=v2", "--branch", "--untracked-files=no", "-z")
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(state, "# branch.oid (initial)\x00") {
			return "", headErr
		}
		empty, err := r.Exec(ctx, dir, "hash-object", "-t", "tree", "--stdin")
		return strings.TrimSpace(empty), err
	}
	return "HEAD", nil
}
func (r *Repository) WorkingTreeChanges(ctx context.Context, dir, base string) ([]domain.FileStatus, error) {
	raw, err := r.Exec(ctx, dir, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-status", "-z", base, "--")
	if err != nil {
		return nil, err
	}
	tokens := nulPaths(raw)
	if len(tokens)%2 != 0 {
		return nil, fmt.Errorf("malformed working tree name/status output")
	}
	files := make([]domain.FileStatus, 0, len(tokens)/2)
	for i := 0; i < len(tokens); i += 2 {
		files = append(files, domain.FileStatus{Path: tokens[i+1], Staged: tokens[i], Unstaged: " "})
	}
	return files, nil
}
func (r *Repository) Blob(ctx context.Context, dir, hash string) (string, error) {
	return r.Exec(ctx, dir, "cat-file", "blob", hash)
}
func (r *Repository) FileMode(ctx context.Context, dir string) (bool, error) {
	raw, err := r.Exec(ctx, dir, "config", "--type=bool", "--default=true", "--get", "core.filemode")
	return strings.TrimSpace(raw) != "false", err
}
func (r *Repository) HashFile(ctx context.Context, dir, path string) (string, error) {
	raw, err := r.Exec(ctx, dir, "hash-object", "--path="+path, "--", path)
	return strings.TrimSpace(raw), err
}
func (r *Repository) ReadCommitInfo(ctx context.Context, dir, hash string) (*domain.CommitInfo, error) {
	raw, err := r.Exec(ctx, dir, "log", "-1", "--format=%s%x00%P", hash)
	if err != nil {
		return nil, err
	}
	subject, parents, ok := strings.Cut(raw, "\x00")
	if !ok {
		return nil, fmt.Errorf("commit metadata is malformed")
	}
	return &domain.CommitInfo{Subject: strings.TrimSpace(subject), Parents: strings.Fields(parents)}, nil
}
func (r *Repository) CommitFiles(ctx context.Context, dir, hash string, parents []string) ([]string, error) {
	args := []string{"diff-tree", "--root", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z"}
	if len(parents) > 0 {
		args = append(args, parents[0])
	}
	args = append(args, hash, "--")
	raw, err := r.Exec(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return nulPaths(raw), nil
}
func (r *Repository) UpstreamTarget(ctx context.Context, dir, branch string) (string, string, error) {
	raw, err := r.Exec(ctx, dir, "for-each-ref", "--format=%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+branch)
	if err != nil {
		return "", "", err
	}
	remote, ref, ok := strings.Cut(strings.TrimSuffix(raw, "\n"), "\x00")
	if !ok {
		return "", "", fmt.Errorf("invalid upstream metadata")
	}
	return remote, ref, nil
}
func (r *Repository) PushDestinationID(ctx context.Context, dir, remote string) (string, error) {
	raw, err := r.Exec(ctx, dir, "remote", "get-url", "--push", "--all", remote)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))), nil
}
