package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
	"github.com/Rafael-Albernaz-dev/vorq/internal/suggest"
)

// GetCommitChanges reads the current worktree directly, including individual
// untracked files and native rename metadata.
func (a *App) GetCommitChanges(ctx context.Context, currentDir string) (*domain.ChangeSet, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	raw, err := a.gitRunner.Exec(ctx, proj.RootPath, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--renames")
	if err != nil {
		return nil, err
	}
	cs, err := git.ParseStatus(raw, proj.RootPath, "")
	if err != nil {
		return nil, err
	}
	for _, f := range cs.Files {
		if f.Staged == "U" || f.Unstaged == "U" || (f.Staged == "A" && f.Unstaged == "A") || (f.Staged == "D" && f.Unstaged == "D") {
			return nil, fmt.Errorf("resolve merge conflicts before committing: %q", f.Path)
		}
	}
	cs.StagedDiff, err = a.gitRunner.DiffCached(ctx, proj.RootPath)
	if err != nil {
		return nil, fmt.Errorf("read staged diff: %w", err)
	}
	return cs, nil
}

// StageFiles stages workspace-root-relative paths. A single . or -A stages all.
func (a *App) StageFiles(ctx context.Context, currentDir string, paths []string) error {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return err
	}
	if err := validateCommitPaths(paths); err != nil {
		return err
	}
	return a.gitRunner.Add(ctx, proj.RootPath, paths...)
}

// UnstageFiles changes only the index; it never discards working files.
func (a *App) UnstageFiles(ctx context.Context, currentDir string, paths []string) error {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return err
	}
	if err := validateCommitPaths(paths); err != nil {
		return err
	}
	return a.gitRunner.Reset(ctx, proj.RootPath, paths...)
}

func validateCommitPaths(paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	for _, path := range paths {
		clean := filepath.Clean(path)
		if path == "" || filepath.IsAbs(path) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("path must be relative to the workspace root: %q", path)
		}
	}
	return nil
}

func stagedFiles(cs *domain.ChangeSet) []domain.FileStatus {
	var files []domain.FileStatus
	for _, f := range cs.Files {
		if f.Staged != "" && f.Staged != " " && f.Staged != "?" {
			files = append(files, f)
		}
	}
	return files
}

// ValidateCommitType shares type validation across interfaces and use cases.
func ValidateCommitType(commitType string) error {
	if !suggest.ValidType(commitType) {
		return fmt.Errorf("invalid commit type %q (feat, fix, chore, refactor, docs, test, style, perf)", commitType)
	}
	return nil
}

// ProposeCommit derives its message exclusively from the current index.
func (a *App) ProposeCommit(ctx context.Context, currentDir, commitType string) (*domain.CommitProposal, error) {
	_, proposal, err := a.PreviewCommit(ctx, currentDir, commitType, false)
	return proposal, err
}

// PreviewCommit is read-only, even with all=true. That mode compares HEAD to
// working files and includes non-ignored untracked files, without running add.
func (a *App) PreviewCommit(ctx context.Context, currentDir, commitType string, all bool) (*domain.ChangeSet, *domain.CommitProposal, error) {
	if err := ValidateCommitType(commitType); err != nil {
		return nil, nil, err
	}
	cs, err := a.GetCommitChanges(ctx, currentDir)
	if err != nil {
		return nil, nil, err
	}
	files := stagedFiles(cs)
	if all {
		base := "HEAD"
		if _, headErr := a.gitRunner.RevParseHead(ctx, cs.WorkspacePath); headErr != nil {
			// Only an unborn branch permits comparison against the empty tree.
			state, err := a.gitRunner.Exec(ctx, cs.WorkspacePath, "--no-optional-locks", "status", "--porcelain=v2", "--branch", "--untracked-files=no", "-z")
			if err != nil {
				return nil, nil, err
			}
			if !strings.HasPrefix(state, "# branch.oid (initial)\x00") {
				return nil, nil, headErr
			}
			emptyTree, err := a.gitRunner.Exec(ctx, cs.WorkspacePath, "hash-object", "-t", "tree", "--stdin")
			if err != nil {
				return nil, nil, err
			}
			base = strings.TrimSpace(emptyTree)
		}
		raw, err := a.gitRunner.Exec(ctx, cs.WorkspacePath, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-status", "-z", base, "--")
		if err != nil {
			return nil, nil, err
		}
		byPath := map[string]domain.FileStatus{}
		tokens := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
		for i := 0; i+1 < len(tokens); i += 2 {
			byPath[tokens[i+1]] = domain.FileStatus{Path: tokens[i+1], Staged: tokens[i], Unstaged: " "}
		}
		for _, f := range cs.Files {
			if f.Staged == "?" {
				status, err := a.previewUntracked(ctx, cs.WorkspacePath, base, f.Path)
				if err != nil {
					return nil, nil, err
				}
				if status == "" {
					delete(byPath, f.Path)
				} else {
					byPath[f.Path] = domain.FileStatus{Path: f.Path, Staged: status, Unstaged: " "}
				}
			}
		}
		files = nil
		for _, file := range byPath {
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	preview := &domain.ChangeSet{WorkspacePath: cs.WorkspacePath, Files: files}
	if !all {
		preview.StagedDiff = cs.StagedDiff
	}
	if len(files) == 0 {
		return preview, nil, fmt.Errorf("no staged files to commit")
	}
	return preview, suggest.ProposeMessage(commitType, files), nil
}

// A staged deletion followed by recreation is reported as D plus ?? by status.
// Compare the recreated file to HEAD so --all previews M (or no net change),
// rather than a misleading deletion/addition pair. hash-object without -w only
// reads content; no objects or alternate index are written.
func (a *App) previewUntracked(ctx context.Context, dir, base, path string) (string, error) {
	entry, err := a.gitRunner.Exec(ctx, dir, "--literal-pathspecs", "ls-tree", "-z", base, "--", path)
	if err != nil {
		return "", err
	}
	if entry == "" {
		return "A", nil
	}
	metadata, _, ok := strings.Cut(entry, "\t")
	fields := strings.Fields(metadata)
	if !ok || len(fields) != 3 {
		return "", fmt.Errorf("invalid tree entry for %q", path)
	}
	info, err := os.Lstat(filepath.Join(dir, path))
	if err != nil {
		return "", err
	}
	mode := "100644"
	var matches bool
	if info.Mode()&os.ModeSymlink != 0 {
		mode = "120000"
		target, err := os.Readlink(filepath.Join(dir, path))
		if err != nil {
			return "", err
		}
		original, err := a.gitRunner.Exec(ctx, dir, "cat-file", "blob", fields[2])
		if err != nil {
			return "", err
		}
		matches = target == original
	} else if info.Mode().IsRegular() {
		if info.Mode()&0111 != 0 {
			mode = "100755"
		}
		fileMode, err := a.gitRunner.Exec(ctx, dir, "config", "--type=bool", "--default=true", "--get", "core.filemode")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(fileMode) == "false" && (fields[0] == "100644" || fields[0] == "100755") {
			mode = fields[0]
		}
		hash, err := a.gitRunner.Exec(ctx, dir, "hash-object", "--path="+path, "--", path)
		if err != nil {
			return "", err
		}
		matches = strings.TrimSpace(hash) == fields[2]
	} else {
		return "", fmt.Errorf("cannot preview non-regular untracked path %q", path)
	}
	if mode != fields[0] && (mode == "120000" || fields[0] == "120000") {
		return "T", nil
	}
	if matches && mode == fields[0] {
		return "", nil
	}
	return "M", nil
}

// CreateReviewedCommit refuses an index that changed since the user's review.
func (a *App) CreateReviewedCommit(ctx context.Context, currentDir, message, expectedDiff string) (*domain.CommitResult, error) {
	cs, err := a.GetCommitChanges(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	if cs.StagedDiff != expectedDiff {
		return nil, fmt.Errorf("o stage mudou após a revisão; revise os arquivos e confirme novamente")
	}
	return a.CreateCommit(ctx, currentDir, message)
}

// CreateCommit validates the index and delegates hooks/signing to native Git.
func (a *App) CreateCommit(ctx context.Context, currentDir, message string) (*domain.CommitResult, error) {
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("commit message must not be empty")
	}
	cs, err := a.GetCommitChanges(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	if cs.StagedCount() == 0 {
		return nil, fmt.Errorf("no staged files to commit")
	}
	hash, err := a.gitRunner.Commit(ctx, cs.WorkspacePath, message)
	if err != nil {
		return nil, err
	}
	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	result := &domain.CommitResult{Hash: hash, ShortHash: shortHash}
	metadataFailure := func(operation string, cause error) (*domain.CommitResult, error) {
		return result, &domain.CommitMetadataError{Hash: hash, Operation: operation, Err: cause}
	}
	// Read the resulting commit, because native hooks may have edited the message
	// or index. Metadata must describe what Git actually committed.
	metadata, err := a.gitRunner.Exec(ctx, cs.WorkspacePath, "log", "-1", "--format=%s%x00%P", hash)
	if err != nil {
		return metadataFailure("reading subject", err)
	}
	subject, parentList, ok := strings.Cut(metadata, "\x00")
	if !ok {
		return metadataFailure("reading subject", fmt.Errorf("commit metadata is malformed"))
	}
	result.Subject = strings.TrimSpace(subject)
	args := []string{"diff-tree", "--root", "--no-commit-id", "--name-only", "--no-renames", "-r", "-z"}
	if parents := strings.Fields(parentList); len(parents) > 0 {
		args = append(args, parents[0])
	}
	args = append(args, hash, "--")
	raw, err := a.gitRunner.Exec(ctx, cs.WorkspacePath, args...)
	if err != nil {
		return metadataFailure("reading files", err)
	}
	files := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
	if raw == "" {
		files = nil
	}
	result.FilesCommitted = files
	return result, nil
}
