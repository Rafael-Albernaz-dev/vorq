package git

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func (r *Repository) requireIdleRepository(ctx context.Context, dir string) error {
	if os.Getenv("GIT_INDEX_FILE") != "" {
		return fmt.Errorf("alternate GIT_INDEX_FILE is active; use the workspace's normal index")
	}
	gitDir, err := r.Exec(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer"} {
		_, err := os.Lstat(filepath.Join(strings.TrimSpace(gitDir), marker))
		if err == nil {
			return fmt.Errorf("Git operation in progress (%s); finish it before organizing commits", marker)
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (r *Repository) workingMode(ctx context.Context, dir, path string, info os.FileInfo) (string, error) {
	mode := "100644"
	if info.Mode()&0111 != 0 {
		mode = "100755"
	}
	setting, err := r.Exec(ctx, dir, "config", "--get", "core.filemode")
	if err == nil && strings.TrimSpace(setting) == "false" {
		entry, e := r.TreeEntry(ctx, dir, "HEAD", path)
		if e != nil {
			return "", e
		}
		if entry != nil && (entry.Mode == "100644" || entry.Mode == "100755") {
			mode = entry.Mode
		}
	}
	return mode, nil
}

func (r *Repository) input(ctx context.Context, dir string, input io.Reader, env map[string]string, args ...string) (string, error) {
	n, ok := r.Runner.(interface {
		ExecInput(context.Context, string, io.Reader, map[string]string, ...string) (string, error)
	})
	if !ok {
		return "", fmt.Errorf("native scoped Git execution unavailable")
	}
	return n.ExecInput(ctx, dir, input, env, args...)
}
func (r *Repository) indexFingerprint(ctx context.Context, dir string) (string, error) {
	raw, err := r.Exec(ctx, dir, "ls-files", "--stage", "-z")
	return digest([]byte(raw)), err
}

// CaptureSnapshot reads working versions explicitly; the original index is never
// staged/reset by analysis. Content is pinned in the snapshot before approval.
func (r *Repository) CaptureSnapshot(ctx context.Context, dir string) (*domain.ChangeSnapshot, error) {
	root, _, err := r.ProjectPaths(ctx, dir)
	if err != nil {
		return nil, err
	}
	if err = r.requireIdleRepository(ctx, root); err != nil {
		return nil, err
	}
	head, err := r.RevParseHead(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("create an initial commit before organizing commits")
	}
	index, err := r.indexFingerprint(ctx, root)
	if err != nil {
		return nil, err
	}
	cs, err := r.ReadChanges(ctx, root, "")
	if err != nil {
		return nil, err
	}
	snapshot := &domain.ChangeSnapshot{Workspace: root, Head: head, Index: index}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	for i, status := range cs.Files {
		if status.IsConflicted() {
			return nil, fmt.Errorf("resolve conflicts before analysis")
		}
		file := domain.SnapshotFile{ID: fmt.Sprintf("F%04d", i+1), Path: status.Path, OldPath: status.OrigPath}
		if !utf8.ValidString(file.Path) || !utf8.ValidString(file.OldPath) {
			return nil, fmt.Errorf("non-UTF-8 path requires manual handling")
		}
		if filepath.IsAbs(file.Path) || strings.Contains("/"+filepath.ToSlash(file.Path)+"/", "/../") || strings.Contains("/"+file.Path+"/", "/.git/") {
			return nil, fmt.Errorf("unsafe snapshot path")
		}
		ignored, e := r.IgnoredPath(ctx, root, file.Path)
		if e != nil {
			return nil, e
		}
		if ignored {
			file.Excluded = "ignored by Git; not sent to AI"
		}
		info, e := fs.Lstat(file.Path)
		if os.IsNotExist(e) {
			entry, err := r.TreeEntry(ctx, root, "HEAD", file.Path)
			if err != nil {
				return nil, err
			}
			if entry == nil {
				return nil, fmt.Errorf("%q exists only in the stage; restore the working file or handle it manually", file.Path)
			}
			file.Mode = "0"
			file.Fingerprint = "deleted"
			if entry.Mode != "100644" && entry.Mode != "100755" {
				file.Excluded = "deleted symlink/submodule: handle manually"
			}
		} else if e != nil {
			return nil, e
		} else if !info.Mode().IsRegular() {
			file.Excluded = "symlink, directory/submodule or special file: handle manually"
			file.Fingerprint = info.Mode().String()
		} else {
			file.Mode, e = r.workingMode(ctx, root, file.Path, info)
			if e != nil {
				return nil, e
			}
			if info.Size() > 256*1024 {
				file.Excluded = "file exceeds 256 KiB: handle manually"
				file.Fingerprint = fmt.Sprintf("large:%d:%d", info.Size(), info.ModTime().UnixNano())
			} else {
				data, err := fs.ReadFile(file.Path)
				if err != nil {
					return nil, err
				}
				file.Fingerprint = digest(append([]byte(file.Mode+":"), data...))
				if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
					file.Excluded = "binary: handle manually"
				}
				if file.Excluded == "" {
					file.Content = data
				}
			}
		}
		if file.Excluded == "" {
			args := []string{"--literal-pathspecs", "diff", "HEAD", "--no-ext-diff", "--no-textconv", "--color=never", "--", file.Path}
			if file.OldPath != "" {
				args = append(args, file.OldPath)
			}
			file.Diff, err = r.Exec(ctx, root, args...)
			if err != nil {
				return nil, err
			}
			if status.IsUntracked() {
				file.Diff = "New file " + file.Path + "\n" + string(file.Content)
			}
			if strings.TrimSpace(file.Diff) == "" {
				file.Excluded = "no net working-tree change from HEAD; existing stage preserved"
			}
		}
		snapshot.Files = append(snapshot.Files, file)
	}
	sort.Slice(snapshot.Files, func(i, j int) bool { return snapshot.Files[i].Path < snapshot.Files[j].Path })
	var identity strings.Builder
	identity.WriteString(root + head + index)
	for _, f := range snapshot.Files {
		identity.WriteString(f.Path + f.OldPath + f.Fingerprint)
	}
	snapshot.ID = digest([]byte(identity.String()))
	if err = r.ValidateSnapshot(ctx, *snapshot, head, index); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *Repository) ValidateSnapshot(ctx context.Context, s domain.ChangeSnapshot, head, index string) error {
	if err := r.requireIdleRepository(ctx, s.Workspace); err != nil {
		return err
	}
	current, e := r.RevParseHead(ctx, s.Workspace)
	if e != nil {
		return e
	}
	if current != head {
		return fmt.Errorf("HEAD changed; review a new plan")
	}
	current, e = r.indexFingerprint(ctx, s.Workspace)
	if e != nil {
		return e
	}
	if current != index {
		return fmt.Errorf("stage changed; review a new plan")
	}
	root, e := os.OpenRoot(s.Workspace)
	if e != nil {
		return e
	}
	defer root.Close()
	for _, f := range s.Files {
		if f.Excluded != "" {
			continue
		}
		info, e := root.Lstat(f.Path)
		if f.Mode == "0" {
			if !os.IsNotExist(e) {
				return fmt.Errorf("working file changed: %q", f.Path)
			}
			continue
		}
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("working file changed: %q", f.Path)
		}
		data, e := root.ReadFile(f.Path)
		if e != nil {
			return e
		}
		mode, e := r.workingMode(ctx, s.Workspace, f.Path, info)
		if e != nil {
			return e
		}
		if digest(append([]byte(mode+":"), data...)) != f.Fingerprint {
			return fmt.Errorf("working file changed: %q", f.Path)
		}
	}
	return nil
}

// CommitSnapshot holds the real index lock, commits a pinned private index with
// native hooks/signing, then atomically reconciles only the approved paths.
// Already created commits are always reported, even if reconciliation fails.
func (r *Repository) CommitSnapshot(ctx context.Context, s domain.ChangeSnapshot, group domain.PlannedCommit, head, index string) (hash, nextIndex string, err error) {
	indexPath, e := r.Exec(ctx, s.Workspace, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if e != nil {
		return "", "", e
	}
	indexPath = strings.TrimSpace(indexPath)
	lockPath := indexPath + ".lock"
	lock, e := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", "", fmt.Errorf("index is busy; retry after the other Git operation: %w", e)
	}
	owned := true
	defer func() {
		lock.Close()
		if owned {
			os.Remove(lockPath)
		}
	}()
	if e = r.ValidateSnapshot(ctx, s, head, index); e != nil {
		return "", "", e
	}
	original, e := os.ReadFile(indexPath)
	if e != nil && !os.IsNotExist(e) {
		return "", "", e
	}
	dir, e := os.MkdirTemp(filepath.Dir(indexPath), "vorq-plan-")
	if e != nil {
		return "", "", e
	}
	defer os.RemoveAll(dir)
	private := filepath.Join(dir, "commit-index")
	env := map[string]string{"GIT_INDEX_FILE": private}
	if _, e = r.input(ctx, s.Workspace, nil, env, "read-tree", head); e != nil {
		return "", "", e
	}
	selected := map[string]bool{}
	for _, id := range group.Files {
		selected[id] = true
	}
	var updates strings.Builder
	for _, f := range s.Files {
		if !selected[f.ID] {
			continue
		}
		oid := strings.Repeat("0", len(head))
		if f.Mode != "0" {
			out, e := r.input(ctx, s.Workspace, bytes.NewReader(f.Content), nil, "hash-object", "-w", "--path="+f.Path, "--stdin")
			if e != nil {
				return "", "", e
			}
			oid = strings.TrimSpace(out)
		}
		updates.WriteString(f.Mode + " " + oid + "\t" + f.Path + "\x00")
		if f.OldPath != "" && f.OldPath != f.Path {
			updates.WriteString("0 " + strings.Repeat("0", len(head)) + "\t" + f.OldPath + "\x00")
		}
	}
	if _, e = r.input(ctx, s.Workspace, strings.NewReader(updates.String()), env, "update-index", "-z", "--index-info"); e != nil {
		return "", "", e
	}
	expectedTree, e := r.input(ctx, s.Workspace, nil, env, "write-tree")
	if e != nil {
		return "", "", e
	}
	message := group.Subject
	if group.Body != "" {
		message += "\n\n" + group.Body
	}
	_, commitErr := r.input(ctx, s.Workspace, nil, env, "commit", "-m", message)
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	actualHead, headErr := r.RevParseHead(cleanup, s.Workspace)
	if headErr != nil {
		return "", "", &domain.CommitMetadataError{Operation: "HEAD verification (inspect before retry)", Err: errors.Join(commitErr, headErr)}
	}
	if actualHead == head {
		return "", "", commitErr
	}
	hash = actualHead
	parent, e := r.Exec(cleanup, s.Workspace, "rev-parse", hash+"^")
	if e != nil || strings.TrimSpace(parent) != head {
		return hash, "", fmt.Errorf("HEAD changed concurrently; inspect history/index before retry")
	}
	actualTree, e := r.Exec(cleanup, s.Workspace, "rev-parse", hash+"^{tree}")
	if e != nil || strings.TrimSpace(actualTree) != strings.TrimSpace(expectedTree) {
		return hash, "", fmt.Errorf("commit %s exists but hooks changed its content; stop and inspect stage/history", hash)
	}
	// The private index contains exactly HEAD + approved content. Apply the same
	// entries to a copy of the original real index, preserving all other entries.
	syncIndex := filepath.Join(dir, "sync-index")
	if len(original) > 0 {
		if e = os.WriteFile(syncIndex, original, 0600); e != nil {
			return hash, "", e
		}
	} else {
		if _, e = r.input(cleanup, s.Workspace, nil, map[string]string{"GIT_INDEX_FILE": syncIndex}, "read-tree", head); e != nil {
			return hash, "", e
		}
	}
	if _, e = r.input(cleanup, s.Workspace, strings.NewReader(updates.String()), map[string]string{"GIT_INDEX_FILE": syncIndex}, "update-index", "-z", "--index-info"); e != nil {
		return hash, "", e
	}
	updated, e := os.ReadFile(syncIndex)
	if e != nil {
		return hash, "", e
	}
	if _, e = lock.Write(updated); e != nil {
		return hash, "", e
	}
	if e = lock.Sync(); e != nil {
		return hash, "", e
	}
	if e = lock.Close(); e != nil {
		return hash, "", e
	}
	if e = os.Rename(lockPath, indexPath); e != nil {
		return hash, "", e
	}
	owned = false
	nextIndex, e = r.indexFingerprint(cleanup, s.Workspace)
	if e != nil {
		return hash, "", e
	}
	if commitErr != nil {
		return hash, nextIndex, fmt.Errorf("commit exists but native output failed: %w", commitErr)
	}
	return hash, nextIndex, nil
}
