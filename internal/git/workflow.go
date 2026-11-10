package git

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// WebRepositoryURL never returns credentials, query parameters or fragments.
func WebRepositoryURL(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "git@") && !strings.Contains(raw, "://") {
		host, path, ok := strings.Cut(strings.TrimPrefix(raw, "git@"), ":")
		if !ok {
			return "", ""
		}
		raw = "https://" + host + "/" + path
	}
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh") {
		return "", ""
	}
	// Only well-formed owner/repository identities are linked. Local paths and
	// custom SSH aliases are intentionally not guessed into public URLs.
	host := strings.ToLower(u.Hostname())
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", ""
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", ""
		}
		for _, c := range p {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
				return "", ""
			}
		}
	}
	for _, c := range host {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
			return "", ""
		}
	}
	return "https://" + host + "/" + path, path
}
func (r *Repository) History(ctx context.Context, dir string, o domain.HistoryOptions) (*domain.HistoryPage, error) {
	refs, e := r.Exec(ctx, dir, "for-each-ref", "--format=%(refname)%00%(objectname)")
	if e != nil {
		return nil, e
	}
	head, headErr := r.RevParseHead(ctx, dir)
	if headErr != nil {
		if _, e := r.PreviewBase(ctx, dir); e != nil {
			return nil, e
		}
	}
	shallow, e := r.Exec(ctx, dir, "rev-parse", "--is-shallow-repository")
	if e != nil {
		return nil, e
	}
	p := &domain.HistoryPage{Skip: o.Skip, Shallow: strings.TrimSpace(shallow) == "true", Snapshot: fmt.Sprintf("%x", sha256.Sum256([]byte(refs+head)))}
	if headErr != nil && refs == "" {
		return p, nil
	}
	args := []string{"--no-pager", "log", "--graph", "--topo-order", "--decorate=short", "--color=never", "--format=%x00%H%x00%P%x00%an%x00%aI%x00%d%x00%s%x00", "--skip=" + strconv.Itoa(o.Skip), "--max-count=" + strconv.Itoa(o.Limit+1)}
	if o.Search != "" {
		args = append(args, "--fixed-strings", "--regexp-ignore-case", "--grep="+o.Search)
	}
	if o.Ref != "" {
		oid, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--end-of-options", o.Ref+"^{commit}")
		if e != nil {
			return nil, e
		}
		args = append(args, strings.TrimSpace(oid))
	} else {
		args = append(args, "--all")
		if headErr == nil {
			args = append(args, head)
		}
	}
	raw, e := r.Exec(ctx, dir, append(args, "--")...)
	if e != nil {
		return nil, e
	}
	for _, line := range strings.Split(strings.TrimSuffix(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) == 1 {
			p.Lines = append(p.Lines, domain.HistoryLine{Graph: line})
			continue
		}
		if len(fields) != 8 {
			return nil, fmt.Errorf("invalid history metadata")
		}
		if p.Count == o.Limit {
			p.HasMore = true
			break
		}
		p.Lines = append(p.Lines, domain.HistoryLine{Graph: fields[0], Hash: fields[1], Parents: fields[2], Author: fields[3], Date: fields[4], Refs: strings.TrimSpace(fields[5]), Subject: fields[6]})
		p.Count++
	}
	return p, nil
}
func (r *Repository) CommitDetail(ctx context.Context, dir, hash string) (*domain.CommitDetail, error) {
	raw, e := r.CommitPreview(ctx, dir, hash)
	if e != nil {
		return nil, e
	}
	return &domain.CommitDetail{Hash: hash, Text: raw}, nil
}

// CommitPreview bounds large patches independently from repository metadata.
// Truncation is visible and never mistaken for complete commit contents.
func (r *NativeRunner) CommitPreview(ctx context.Context, dir, hash string) (string, error) {
	args := []string{"--no-pager", "show", "--no-ext-diff", "--no-textconv", "--color=never", "--format=fuller", "--stat", "--patch", "--diff-merges=first-parent", hash, "--"}
	cmd := exec.CommandContext(ctx, r.gitBinPath, args...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	out, stderr := &cappedOutput{limit: 256 * 1024}, &cappedOutput{limit: 64 * 1024}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if e := cmd.Run(); e != nil {
		return "", fmt.Errorf("leitura do commit: %w: %s", e, stderr.String())
	}
	text := out.String()
	if out.truncated {
		text += "\n\n[Preview limitado a 256 KiB; use git show para o conteúdo completo.]\n"
	}
	return text, nil
}
func (r *Repository) CommitDraft(ctx context.Context, dir, remote, base string) (string, string, error) {
	// Resolve refs before constructing the range; never interpret options or
	// arbitrary revision expressions as a command argument list.
	ref := "refs/remotes/" + remote + "/" + base
	raw, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if e != nil {
		raw, e = r.Exec(ctx, dir, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if e != nil {
			return "", "", fmt.Errorf("base is not available locally; fetch explicitly or choose another base: %w", e)
		}
	}
	oid := strings.TrimSpace(raw)
	messages, e := r.Exec(ctx, dir, "log", "--reverse", "--format=%s%x00%b%x00", oid+"..HEAD", "--")
	if e != nil {
		return "", "", e
	}
	if strings.TrimSpace(messages) == "" {
		return "", "", fmt.Errorf("the branch has no commits relative to the base")
	}
	fields := strings.Split(messages, "\x00")
	title := strings.TrimSpace(fields[0])
	var body []string
	for i := 0; i+1 < len(fields); i += 2 {
		subject := strings.TrimSpace(fields[i])
		if subject != "" {
			body = append(body, "- "+subject)
			if b := strings.TrimSpace(fields[i+1]); b != "" {
				body = append(body, b)
			}
		}
	}
	return title, strings.Join(body, "\n\n"), nil
}
func (r *Repository) PublishedHead(ctx context.Context, dir, remote, branch, oid string) error {
	raw, e := r.Exec(ctx, dir, "ls-remote", "--heads", "--", remote, "refs/heads/"+branch)
	if e != nil {
		return fmt.Errorf("cannot verify the published branch: %w", e)
	}
	fields := strings.Fields(raw)
	if len(fields) != 2 || fields[0] != oid || fields[1] != "refs/heads/"+branch {
		return fmt.Errorf("publish the current branch with the explicit push before creating the PR")
	}
	return nil
}

// MaxBranchFiles bounds how many changed files a PR analysis inspects.
const MaxBranchFiles = 300

// BranchChanges collects commits and per-file diffs of base...HEAD using the
// locally known base (remote-tracking ref first, then the local branch).
func (r *Repository) BranchChanges(ctx context.Context, dir, remote, base string) (*domain.BranchChanges, error) {
	ref := "refs/remotes/" + remote + "/" + base
	raw, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if e != nil {
		ref = base
		if raw, e = r.Exec(ctx, dir, "rev-parse", "--verify", "--end-of-options", base+"^{commit}"); e != nil {
			return nil, fmt.Errorf("base %q is not available locally; fetch explicitly or choose another base: %w", base, e)
		}
	}
	mergeBase, e := r.Exec(ctx, dir, "merge-base", strings.TrimSpace(raw), "HEAD")
	if e != nil {
		return nil, fmt.Errorf("the branch shares no history with %q: %w", base, e)
	}
	mergeBase = strings.TrimSpace(mergeBase)
	out := &domain.BranchChanges{Base: base, BaseRef: ref, MergeBase: mergeBase}
	log, e := r.Exec(ctx, dir, "log", "--reverse", "--format=%H%x00%s%x00%b%x1e", mergeBase+"..HEAD", "--")
	if e != nil {
		return nil, e
	}
	for _, record := range strings.Split(log, "\x1e") {
		fields := strings.SplitN(strings.TrimLeft(record, "\n"), "\x00", 3)
		if len(fields) == 3 && fields[0] != "" {
			out.Commits = append(out.Commits, domain.BranchCommit{Hash: fields[0], Subject: strings.TrimSpace(fields[1]), Body: strings.TrimSpace(fields[2])})
		}
	}
	if len(out.Commits) == 0 {
		return nil, fmt.Errorf("the branch has no commits relative to the base %q", base)
	}
	names, e := r.Exec(ctx, dir, "diff", "--no-ext-diff", "--name-only", "-z", mergeBase, "HEAD", "--")
	if e != nil {
		return nil, e
	}
	paths := nulPaths(names)
	if len(paths) > MaxBranchFiles {
		return nil, fmt.Errorf("the branch changes %d files (limit %d); split the PR or write the description manually (nothing sent)", len(paths), MaxBranchFiles)
	}
	for _, path := range paths {
		diff, e := r.Exec(ctx, dir, "--literal-pathspecs", "diff", "--no-ext-diff", "--no-textconv", "--color=never", mergeBase, "HEAD", "--", path)
		if e != nil {
			return nil, e
		}
		file := domain.BranchFileDiff{Path: path, Diff: diff}
		if len(diff) > MaxAnalysisFile {
			file.Diff = fmt.Sprintf("[diff omitted by vorq: %d bytes, above the %d-byte per-file limit]", len(diff), MaxAnalysisFile)
			file.Omitted = true
		}
		out.Files = append(out.Files, file)
	}
	return out, nil
}

// Primitives for the application's branch and remote policies (VORQ-017):
// the adapter runs Git and parses output; the rules live in internal/app.

// RemoteWebURL reads a remote URL and derives its credential-free web link.
func (r *Repository) RemoteWebURL(ctx context.Context, dir, remote string) (string, string, error) {
	raw, e := r.Exec(ctx, dir, "remote", "get-url", "--", remote)
	if e != nil {
		return "", "", fmt.Errorf("cannot read remote %q", remote)
	}
	url, slug := WebRepositoryURL(raw)
	return url, slug, nil
}

// ValidBranchName reports whether Git accepts name as a branch.
func (r *Repository) ValidBranchName(ctx context.Context, dir, name string) bool {
	_, e := r.Exec(ctx, dir, "check-ref-format", "refs/heads/"+name)
	return e == nil
}

// ResolveCommit resolves a revision to a commit ID without option parsing.
func (r *Repository) ResolveCommit(ctx context.Context, dir, rev string) (string, error) {
	raw, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	return strings.TrimSpace(raw), e
}

// CreateBranchAt creates name at oid, optionally switching to it (without
// overwriting ignored files or recursing into submodules).
func (r *Repository) CreateBranchAt(ctx context.Context, dir, name, oid string, switchTo bool) error {
	var e error
	if switchTo {
		_, e = r.Exec(ctx, dir, "switch", "--no-overwrite-ignore", "--no-recurse-submodules", "--no-track", "-c", name, oid)
	} else {
		_, e = r.Exec(ctx, dir, "branch", "--no-track", "--", name, oid)
	}
	return e
}

// RemoteDefaultBranch reads refs/remotes/<remote>/HEAD (local, no fetch).
func (r *Repository) RemoteDefaultBranch(ctx context.Context, dir, remote string) (string, error) {
	raw, e := r.Exec(ctx, dir, "for-each-ref", "--format=%(symref)", "refs/remotes/"+remote+"/HEAD")
	if e != nil {
		return "", e
	}
	return strings.TrimPrefix(strings.TrimSpace(raw), "refs/remotes/"+remote+"/"), nil
}
