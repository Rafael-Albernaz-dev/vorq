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
func (r *Repository) DeleteRemoteBranch(ctx context.Context, dir, remote, branch, hash string) (string, error) {
	ref := "refs/heads/" + branch
	return r.Exec(ctx, dir, "push", "--porcelain", "--no-recurse-submodules", "--force-with-lease="+ref+":"+hash, "--", remote, ":"+ref)
}
