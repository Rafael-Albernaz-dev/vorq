package git

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
)

// FetchBranch pins one remote branch into a unique refs/vorq/incoming ref and
// refreshes only its remote-tracking ref, without wildcard refmaps or pruning.
func (r *Repository) FetchBranch(ctx context.Context, dir, remote, branch string) (hash, ref string, err error) {
	if _, e := r.Exec(ctx, dir, "check-ref-format", "--branch", branch); e != nil || strings.HasPrefix(branch, "-") || strings.Contains(branch, ":") {
		return "", "", fmt.Errorf("invalid base branch name")
	}
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return "", "", e
	}
	ref = fmt.Sprintf("refs/vorq/incoming/%x", nonce)
	source := "refs/heads/" + branch
	tracking := "refs/remotes/" + remote + "/" + branch
	_, err = r.Exec(ctx, dir, "fetch", "--no-tags", "--no-prune", "--no-prune-tags", "--no-recurse-submodules", "--no-auto-maintenance", "--refmap=", "--", remote, source+":"+ref, source+":"+tracking)
	raw, readErr := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if readErr == nil {
		hash = strings.TrimSpace(raw)
	} else if err == nil {
		err = readErr
	}
	return hash, ref, err
}

// RemoteTrackingHash returns the local refs/remotes/<remote>/<branch> commit,
// or "" when it does not exist. It never touches the network.
func (r *Repository) RemoteTrackingHash(ctx context.Context, dir, remote, branch string) (string, error) {
	raw, e := r.Exec(ctx, dir, "for-each-ref", "--format=%(objectname)", "refs/remotes/"+remote+"/"+branch)
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(raw), nil
}

// CountRange counts commits reachable from to and not from from.
func (r *Repository) CountRange(ctx context.Context, dir, from, to string) (int, error) {
	raw, e := r.Exec(ctx, dir, "rev-list", "--count", "--end-of-options", from+".."+to)
	if e != nil {
		return 0, e
	}
	return strconv.Atoi(strings.TrimSpace(raw))
}

func (r *Repository) MergeInProgress(ctx context.Context, dir string) (bool, error) {
	_, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	return e == nil, nil
}

// MergeBranch merges a pinned commit. Native hooks and signing apply; no
// autostash and ignored files are never overwritten.
func (r *Repository) MergeBranch(ctx context.Context, dir, hash, message string) (string, error) {
	return r.Exec(ctx, dir, "-c", "submodule.recurse=false", "merge", "--no-autostash", "--no-overwrite-ignore", "--no-edit", "--no-stat", "-m", message, hash, "--")
}

func (r *Repository) UnmergedPaths(ctx context.Context, dir string) ([]string, error) {
	raw, e := r.Exec(ctx, dir, "diff", "--name-only", "--diff-filter=U", "-z")
	if e != nil {
		return nil, e
	}
	var paths []string
	for _, p := range strings.Split(raw, "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func (r *Repository) MergeAbort(ctx context.Context, dir string) error {
	_, e := r.Exec(ctx, dir, "merge", "--abort")
	return e
}
