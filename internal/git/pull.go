package git

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"
)

func (r *Repository) PullSourceID(ctx context.Context, dir, remote string) (string, error) {
	raw, e := r.Exec(ctx, dir, "remote", "get-url", "--all", "--", remote)
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw))), nil
}
func (r *Repository) FetchPull(ctx context.Context, dir, remote, branch string) (hash, ref string, err error) {
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return "", "", e
	}
	ref = fmt.Sprintf("refs/vorq/incoming/%x", nonce)
	// Restrict tracking updates to the reviewed upstream, without configured
	// wildcard refmaps or implicit pruning. The unique ref pins this fetch even
	// when another terminal overwrites FETCH_HEAD concurrently.
	tracking, e := r.Exec(ctx, dir, "rev-parse", "--symbolic-full-name", "@{upstream}")
	if e != nil {
		return "", "", e
	}
	tracking = strings.TrimSpace(tracking)
	if !strings.HasPrefix(tracking, "refs/remotes/") {
		return "", "", fmt.Errorf("upstream remoto inválido")
	}
	source := "refs/heads/" + branch
	_, err = r.Exec(ctx, dir, "fetch", "--no-tags", "--no-prune", "--no-prune-tags", "--no-recurse-submodules", "--no-auto-maintenance", "--refmap=", "--", remote, source+":"+ref, source+":"+tracking)
	raw, readErr := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if readErr == nil {
		hash = strings.TrimSpace(raw)
	} else if err == nil {
		err = readErr
	}
	return hash, ref, err
}
func (r *Repository) ApplyPull(ctx context.Context, dir, hash string) (string, error) {
	return r.Exec(ctx, dir, "-c", "submodule.recurse=false", "merge", "--ff-only", "--no-autostash", "--no-overwrite-ignore", "--no-edit", "--no-stat", hash, "--")
}
func (r *Repository) DropPullRef(ctx context.Context, dir, ref, hash string) error {
	if !strings.HasPrefix(ref, "refs/vorq/incoming/") {
		return fmt.Errorf("referência de atualização inválida")
	}
	args := []string{"update-ref", "-d", ref}
	if hash != "" {
		args = append(args, hash)
	}
	_, e := r.Exec(ctx, dir, args...)
	return e
}
