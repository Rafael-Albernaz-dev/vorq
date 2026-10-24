package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// Runner defines the interface for interacting with the native Git binary.
type Runner interface {
	Exec(ctx context.Context, dir string, args ...string) (string, error)
	Version(ctx context.Context) (string, error)
	ListWorktrees(ctx context.Context, repoDir string) (string, error)
	Status(ctx context.Context, wsDir string) (string, error)
	AheadBehind(ctx context.Context, wsDir string) (int, int, error)
	DiffCached(ctx context.Context, wsDir string) (string, error)
	Diff(ctx context.Context, wsDir string) (string, error)
	BranchExists(ctx context.Context, repoDir string, branch string) (bool, error)
	WorktreeAdd(ctx context.Context, repoDir, branch, targetPath, baseBranch string, checkoutExisting bool) error
	WorktreeRemove(ctx context.Context, repoDir, targetPath string, force bool) error
	WorktreePrune(ctx context.Context, repoDir string) (string, error)
	BranchDelete(ctx context.Context, repoDir, branch string, force bool) error
	IgnoredFiles(ctx context.Context, wsDir string) ([]string, error)
	Add(ctx context.Context, wsDir string, paths ...string) error
	Reset(ctx context.Context, wsDir string, paths ...string) error
	Commit(ctx context.Context, wsDir string, message string) (string, error)
	RevParseHead(ctx context.Context, wsDir string) (string, error)
	CurrentBranch(ctx context.Context, wsDir string) (string, error)
	Remotes(ctx context.Context, wsDir string) ([]string, error)
	Upstream(ctx context.Context, wsDir string) (string, error)
	Push(ctx context.Context, wsDir, remote, branch, target string, setUpstream bool) (string, error)
	FileDiff(ctx context.Context, wsDir, path string, staged bool) (string, bool, error)
}

// NativeRunner executes Git via system subprocesses.
type NativeRunner struct {
	gitBinPath string
}

// NewNativeRunner creates a new NativeRunner looking for the 'git' binary in PATH.
func NewNativeRunner() (*NativeRunner, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git binary not found in system PATH: %w", err)
	}
	return &NativeRunner{gitBinPath: path}, nil
}

// Exec executes a git command in the specified directory with structured arguments.
func (r *NativeRunner) Exec(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.gitBinPath, args...)
	cmd.WaitDelay = 2 * time.Second
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := strings.TrimSpace(stderr.String())
		if errStr == "" {
			errStr = strings.TrimSpace(stdout.String())
		}
		if errStr == "" {
			errStr = err.Error()
		}
		return "", fmt.Errorf("git error [%v]: %s: %w", args, errStr, err)
	}

	return stdout.String(), nil
}

// Version returns the output of `git --version`.
func (r *NativeRunner) Version(ctx context.Context) (string, error) {
	out, err := r.Exec(ctx, "", "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ListWorktrees executes `git worktree list --porcelain -z`.
func (r *NativeRunner) ListWorktrees(ctx context.Context, repoDir string) (string, error) {
	return r.Exec(ctx, repoDir, "worktree", "list", "--porcelain", "-z")
}

// Status executes `git status --porcelain -z`.
func (r *NativeRunner) Status(ctx context.Context, wsDir string) (string, error) {
	return r.Exec(ctx, wsDir, "status", "--porcelain", "-z")
}

// AheadBehind queries commits ahead and behind upstream. Returns (0, 0, nil) if no upstream is set.
func (r *NativeRunner) AheadBehind(ctx context.Context, wsDir string) (int, int, error) {
	out, err := r.Exec(ctx, wsDir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		// Typically means no upstream configured, which is normal for local branches.
		return 0, 0, nil
	}

	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return 0, 0, nil
	}

	var ahead, behind int
	fmt.Sscanf(fields[0], "%d", &ahead)
	fmt.Sscanf(fields[1], "%d", &behind)
	return ahead, behind, nil
}

// DiffCached returns staged diff.
func (r *NativeRunner) DiffCached(ctx context.Context, wsDir string) (string, error) {
	return r.Exec(ctx, wsDir, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--color=never")
}

// Add stages literal paths, or the entire workspace for the single sentinel . or -A.
func (r *NativeRunner) Add(ctx context.Context, wsDir string, paths ...string) error {
	if len(paths) == 0 {
		return fmt.Errorf("at least one path is required for stage")
	}
	for _, path := range paths {
		if path == "." || path == "-A" {
			if len(paths) != 1 {
				return fmt.Errorf("stage-all sentinel must be used alone")
			}
			_, err := r.Exec(ctx, wsDir, "add", "-A")
			return err
		}
		if path == "" {
			return fmt.Errorf("empty stage path")
		}
	}
	_, err := r.Exec(ctx, wsDir, append([]string{"--literal-pathspecs", "add", "--"}, paths...)...)
	return err
}

// Reset removes literal paths from the index without changing working files.
func (r *NativeRunner) Reset(ctx context.Context, wsDir string, paths ...string) error {
	if len(paths) == 0 {
		return fmt.Errorf("at least one path is required for unstage")
	}
	for _, path := range paths {
		if path == "" {
			return fmt.Errorf("empty unstage path")
		}
	}
	// An unborn branch has no HEAD to restore. Removing only index entries
	// preserves every working byte, including newly staged files.
	if _, headErr := r.Exec(ctx, wsDir, "rev-parse", "--verify", "--quiet", "HEAD"); headErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exitErr *exec.ExitError
		if !errors.As(headErr, &exitErr) || exitErr.ExitCode() != 1 {
			return headErr
		}
		if _, err := r.Exec(ctx, wsDir, "symbolic-ref", "--quiet", "HEAD"); err != nil {
			return headErr
		}
		_, err := r.Exec(ctx, wsDir, append([]string{"--literal-pathspecs", "rm", "--cached", "-f", "-r", "--ignore-unmatch", "--"}, paths...)...)
		return err
	}
	_, err := r.Exec(ctx, wsDir, append([]string{"--literal-pathspecs", "reset", "--"}, paths...)...)
	return err
}

// Commit preserves native hooks and signing configuration; it never pushes.
func (r *NativeRunner) Commit(ctx context.Context, wsDir string, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message must not be empty")
	}
	if _, err := r.Exec(ctx, wsDir, "commit", "-m", message); err != nil {
		return "", err
	}
	hash, err := r.RevParseHead(ctx, wsDir)
	if err != nil {
		// Native commit already succeeded. A failed metadata read must never be
		// presented as permission to retry creating the commit.
		return "", &domain.CommitMetadataError{Operation: "resolving HEAD", Err: err}
	}
	return hash, nil
}

// RevParseHead returns the full object ID of the current commit.
func (r *NativeRunner) RevParseHead(ctx context.Context, wsDir string) (string, error) {
	out, err := r.Exec(ctx, wsDir, "rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(out), err
}

func (r *NativeRunner) CurrentBranch(ctx context.Context, wsDir string) (string, error) {
	out, err := r.Exec(ctx, wsDir, "symbolic-ref", "--quiet", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

func (r *NativeRunner) Remotes(ctx context.Context, wsDir string) ([]string, error) {
	out, err := r.Exec(ctx, wsDir, "remote")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSpace(out), "\n"), nil
}

func (r *NativeRunner) Upstream(ctx context.Context, wsDir string) (string, error) {
	out, err := r.Exec(ctx, wsDir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	return strings.TrimSpace(out), err
}

// Push uses an explicit refspec so push.default, configured push refspecs and
// differently named upstream branches cannot silently redirect publication.
func (r *NativeRunner) Push(ctx context.Context, wsDir, remote, branch, target string, setUpstream bool) (string, error) {
	if remote == "" || branch == "" || target == "" {
		return "", fmt.Errorf("remote and branch names are required")
	}
	args := []string{"push", "--porcelain", "--no-follow-tags", "--recurse-submodules=no"}
	if setUpstream {
		args = append(args, "-u")
	}
	args = append(args, "--", remote, "refs/heads/"+branch+":refs/heads/"+target)
	return r.Exec(ctx, wsDir, args...)
}

// cappedOutput drains output without retaining more than the preview limit.
type cappedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.buffer.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func (b *cappedOutput) String() string { return b.buffer.String() }

func (r *NativeRunner) FileDiff(ctx context.Context, wsDir, path string, staged bool) (string, bool, error) {
	args := []string{"--no-pager", "--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--color=never"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)
	cmd := exec.CommandContext(ctx, r.gitBinPath, args...)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = wsDir
	out := &cappedOutput{limit: 256 * 1024}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = out, &stderr
	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("read file diff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), out.truncated, nil
}

// Diff returns unstaged diff.
func (r *NativeRunner) Diff(ctx context.Context, wsDir string) (string, error) {
	return r.Exec(ctx, wsDir, "diff", "--no-ext-diff", "--no-textconv", "--color=never")
}

// BranchExists checks if a local branch already exists in the repository.
func (r *NativeRunner) BranchExists(ctx context.Context, repoDir string, branch string) (bool, error) {
	_, err := r.Exec(ctx, repoDir, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	return false, nil
}

// WorktreeAdd provisions a new Git worktree.
func (r *NativeRunner) WorktreeAdd(ctx context.Context, repoDir, branch, targetPath, baseBranch string, checkoutExisting bool) error {
	var args []string
	if checkoutExisting {
		args = []string{"worktree", "add", targetPath, branch}
	} else {
		args = []string{"worktree", "add", "-b", branch, targetPath, baseBranch}
	}

	_, err := r.Exec(ctx, repoDir, args...)
	return err
}

// WorktreeRemove tears down a worktree directory.
func (r *NativeRunner) WorktreeRemove(ctx context.Context, repoDir, targetPath string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, targetPath)

	_, err := r.Exec(ctx, repoDir, args...)
	return err
}

// WorktreePrune removes administrative data for worktrees that no longer exist on disk.
func (r *NativeRunner) WorktreePrune(ctx context.Context, repoDir string) (string, error) {
	return r.Exec(ctx, repoDir, "worktree", "prune")
}

// BranchDelete deletes a local branch.
func (r *NativeRunner) BranchDelete(ctx context.Context, repoDir, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := r.Exec(ctx, repoDir, "branch", flag, branch)
	return err
}

// IgnoredFiles returns a list of files or directories ignored by Git in the specified workspace.
func (r *NativeRunner) IgnoredFiles(ctx context.Context, wsDir string) ([]string, error) {
	out, err := r.Exec(ctx, wsDir, "status", "--porcelain=v1", "--ignored", "-z")
	if err != nil {
		return nil, err
	}

	if len(out) == 0 {
		return nil, nil
	}

	tokens := strings.Split(out, "\x00")
	var ignored []string
	for _, t := range tokens {
		if strings.HasPrefix(t, "!! ") {
			path := strings.TrimPrefix(t, "!! ")
			if path != "" && path != ".git/" && path != ".git" {
				ignored = append(ignored, path)
			}
		}
	}
	return ignored, nil
}
