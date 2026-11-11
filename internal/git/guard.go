package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Primitives for the commit/push guardrails (VORQ-018). The rules live in
// internal/app and internal/secrets; this adapter only reads Git objects.

// StagedBlobSizes maps each added/modified staged path to its staged size.
func (r *Repository) StagedBlobSizes(ctx context.Context, dir string) (map[string]int64, error) {
	raw, err := r.Exec(ctx, dir, "diff", "--cached", "--raw", "-z", "--no-renames", "--no-abbrev", "--diff-filter=AMT", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	fields := nulPaths(raw)
	type blob struct{ path, oid string }
	var blobs []blob
	for i := 0; i+1 < len(fields); i += 2 {
		meta := strings.Fields(strings.TrimPrefix(fields[i], ":"))
		if len(meta) < 5 || meta[1] == "160000" || meta[1] == "120000" {
			continue // submodule or symlink: no file content to size
		}
		blobs = append(blobs, blob{fields[i+1], meta[3]})
	}
	sizes := map[string]int64{}
	if len(blobs) == 0 {
		return sizes, nil
	}
	var input strings.Builder
	for _, b := range blobs {
		input.WriteString(b.oid + "\n")
	}
	out, err := r.ExecInput(ctx, dir, strings.NewReader(input.String()), nil, "cat-file", "--batch-check=%(objectsize)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != len(blobs) {
		return nil, fmt.Errorf("unexpected object size output")
	}
	for i, b := range blobs {
		n, err := strconv.ParseInt(strings.TrimSpace(lines[i]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected object size for %q", b.path)
		}
		sizes[b.path] = n
	}
	return sizes, nil
}

// UnpublishedCommits lists commits reachable from HEAD that no remote-tracking
// ref contains (covers a new branch without upstream), at most limit+1 so the
// caller can tell the list was cut.
func (r *Repository) UnpublishedCommits(ctx context.Context, dir string, limit int) ([]string, error) {
	raw, err := r.Exec(ctx, dir, "rev-list", "--max-count="+strconv.Itoa(limit+1), "HEAD", "--not", "--remotes", "--")
	if err != nil {
		return nil, err
	}
	return strings.Fields(raw), nil
}

// CommitPatch is the patch a commit introduces relative to its first parent
// (the full tree for a root commit). Merge commits produce no patch: the
// commits they bring in are evaluated individually.
func (r *Repository) CommitPatch(ctx context.Context, dir, hash string) (string, error) {
	return r.Exec(ctx, dir, "show", "--format=", "--patch", "--no-ext-diff", "--no-textconv", "--color=never", "--no-renames", "--diff-merges=off", "--end-of-options", hash, "--")
}
