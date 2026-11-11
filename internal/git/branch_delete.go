package git

import (
	"context"
	"strconv"
	"strings"
)

// UnpublishedCount counts commits reachable from hash and from no remote.
func (r *Repository) UnpublishedCount(ctx context.Context, dir, hash string) (int, error) {
	raw, e := r.Exec(ctx, dir, "rev-list", "--count", hash, "--not", "--remotes")
	if e != nil {
		return 0, e
	}
	return strconv.Atoi(strings.TrimSpace(raw))
}

// LocalBranchHash returns refs/heads/<name>, or "" when it does not exist.
func (r *Repository) LocalBranchHash(ctx context.Context, dir, name string) (string, error) {
	raw, e := r.Exec(ctx, dir, "for-each-ref", "--format=%(objectname)", "refs/heads/"+name)
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(raw), nil
}

// DeleteRemoteBranch deletes refs/heads/<branch> on remote only while it still
// points at the reviewed hash (lease), so newer remote commits are never lost.
// A branch already gone on the remote (e.g. GitHub's auto-delete after a PR
// merge) is not an error: only the stale remote-tracking ref is removed.
func (r *Repository) DeleteRemoteBranch(ctx context.Context, dir, remote, branch, hash string) (string, error) {
	ref := "refs/heads/" + branch
	out, err := r.Exec(ctx, dir, "push", "--porcelain", "--no-recurse-submodules", "--force-with-lease="+ref+":"+hash, "--", remote, ":"+ref)
	if err == nil {
		return out, nil
	}
	if heads, e := r.Exec(ctx, dir, "ls-remote", "--heads", "--", remote, ref); e == nil && strings.TrimSpace(heads) == "" {
		_, e = r.Exec(ctx, dir, "update-ref", "-d", "refs/remotes/"+remote+"/"+branch, hash)
		return "already deleted on the remote", e
	}
	return out, err
}
