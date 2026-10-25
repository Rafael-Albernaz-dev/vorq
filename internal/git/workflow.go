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
func (r *Repository) RepositoryIdentity(ctx context.Context, dir string) (*domain.RepositoryLink, error) {
	remotes, e := r.Remotes(ctx, dir)
	if e != nil {
		return nil, e
	}
	link := &domain.RepositoryLink{}
	if len(remotes) == 0 {
		return link, nil
	}
	branch, detached, e := r.Branch(ctx, dir)
	if e != nil {
		return nil, e
	}
	exists := false
	if !detached {
		exists, e = r.BranchExists(ctx, dir, branch)
		if e != nil {
			return nil, e
		}
	}
	if exists {
		remote, _, e := r.UpstreamTarget(ctx, dir, branch)
		if e != nil {
			return nil, e
		}
		for _, name := range remotes {
			if name == remote {
				link.Remote = name
			}
		}
	}
	if link.Remote == "" {
		for _, name := range remotes {
			if name == "origin" {
				link.Remote = name
			}
		}
	}
	if link.Remote == "" && len(remotes) == 1 {
		link.Remote = remotes[0]
	}
	if link.Remote == "" {
		return link, nil
	}
	raw, e := r.Exec(ctx, dir, "remote", "get-url", "--", link.Remote)
	if e != nil {
		return nil, fmt.Errorf("não foi possível ler o remote selecionado")
	}
	link.URL, link.Slug = WebRepositoryURL(raw)
	return link, nil
}
func (r *Repository) CreateBranch(ctx context.Context, dir string, o domain.BranchOptions) error {
	if o.Name == "" || strings.HasPrefix(o.Name, "-") || o.Name == "HEAD" {
		return fmt.Errorf("nome de branch inválido")
	}
	if _, e := r.Exec(ctx, dir, "check-ref-format", "refs/heads/"+o.Name); e != nil {
		return fmt.Errorf("nome de branch inválido")
	}
	exists, e := r.BranchExists(ctx, dir, o.Name)
	if e != nil {
		return e
	}
	if exists {
		return fmt.Errorf("branch %q já existe", o.Name)
	}
	head, e := r.RevParseHead(ctx, dir)
	if e != nil {
		return fmt.Errorf("crie o primeiro commit antes de criar outra branch: %w", e)
	}
	base := head
	if o.Base != "" {
		raw, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--end-of-options", o.Base+"^{commit}")
		if e != nil {
			return fmt.Errorf("base inválida: %w", e)
		}
		base = strings.TrimSpace(raw)
	}
	if !o.Stay && base != head {
		changes, e := r.ReadChanges(ctx, dir, "")
		if e != nil {
			return e
		}
		if !changes.IsClean() {
			return fmt.Errorf("outra base requer workspace limpo; stage e arquivos foram preservados")
		}
	}
	if o.Stay {
		_, e = r.Exec(ctx, dir, "branch", "--no-track", "--", o.Name, base)
	} else {
		_, e = r.Exec(ctx, dir, "switch", "--no-overwrite-ignore", "--no-recurse-submodules", "--no-track", "-c", o.Name, base)
	}
	return e
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
			return nil, fmt.Errorf("metadados de histórico inválidos")
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
	if reader, ok := r.Runner.(interface {
		CommitPreview(context.Context, string, string) (string, error)
	}); ok {
		raw, e := reader.CommitPreview(ctx, dir, hash)
		if e != nil {
			return nil, e
		}
		return &domain.CommitDetail{Hash: hash, Text: raw}, nil
	}
	raw, e := r.Exec(ctx, dir, "--no-pager", "show", "--no-ext-diff", "--no-textconv", "--color=never", "--format=fuller", "--stat", "--patch", "--diff-merges=first-parent", hash, "--")
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
func (r *Repository) DefaultBranch(ctx context.Context, dir string) (string, error) {
	link, e := r.RepositoryIdentity(ctx, dir)
	if e != nil {
		return "", e
	}
	if link.Remote == "" {
		return "", nil
	}
	raw, e := r.Exec(ctx, dir, "for-each-ref", "--format=%(symref)", "refs/remotes/"+link.Remote+"/HEAD")
	if e != nil {
		return "", e
	}
	return strings.TrimPrefix(strings.TrimSpace(raw), "refs/remotes/"+link.Remote+"/"), nil
}
func (r *Repository) CommitDraft(ctx context.Context, dir, base string) (string, string, error) {
	// Resolve refs before constructing the range; never interpret options or
	// arbitrary revision expressions as a command argument list.
	link, e := r.RepositoryIdentity(ctx, dir)
	if e != nil {
		return "", "", e
	}
	ref := "refs/remotes/" + link.Remote + "/" + base
	raw, e := r.Exec(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if e != nil {
		raw, e = r.Exec(ctx, dir, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if e != nil {
			return "", "", fmt.Errorf("base não disponível localmente; faça fetch explícito ou escolha outra base: %w", e)
		}
	}
	oid := strings.TrimSpace(raw)
	messages, e := r.Exec(ctx, dir, "log", "--reverse", "--format=%s%x00%b%x00", oid+"..HEAD", "--")
	if e != nil {
		return "", "", e
	}
	if strings.TrimSpace(messages) == "" {
		return "", "", fmt.Errorf("nenhum commit da branch em relação à base")
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
		return fmt.Errorf("não foi possível conferir a branch publicada: %w", e)
	}
	fields := strings.Fields(raw)
	if len(fields) != 2 || fields[0] != oid || fields[1] != "refs/heads/"+branch {
		return fmt.Errorf("publique a branch atual pelo Push explícito antes de criar o PR")
	}
	return nil
}
