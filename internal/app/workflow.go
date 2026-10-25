package app

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type WorkflowRepository interface {
	RepositoryIdentity(context.Context, string) (*domain.RepositoryLink, error)
	CreateBranch(context.Context, string, domain.BranchOptions) error
	History(context.Context, string, domain.HistoryOptions) (*domain.HistoryPage, error)
	CommitDetail(context.Context, string, string) (*domain.CommitDetail, error)
	DefaultBranch(context.Context, string) (string, error)
	CommitDraft(context.Context, string, string) (string, string, error)
	PublishedHead(context.Context, string, string, string, string) error
}
type PullRequestProvider interface {
	List(context.Context, string, string, domain.PRListOptions) ([]domain.PullRequest, error)
	View(context.Context, string, string, int) (*domain.PullRequest, error)
	Save(context.Context, string, domain.PRDraft) (string, error)
	DefaultBranch(context.Context, string, string) (string, error)
}
type WorkflowService interface {
	GetRepositoryLink(context.Context, string) (*domain.RepositoryLink, error)
	CreateBranch(context.Context, string, domain.BranchOptions) error
	GetHistory(context.Context, string, domain.HistoryOptions) (*domain.HistoryPage, error)
	GetCommitDetail(context.Context, string, string) (*domain.CommitDetail, error)
	ListPullRequests(context.Context, string, domain.PRListOptions) ([]domain.PullRequest, error)
	ViewPullRequest(context.Context, string, int) (*domain.PullRequest, error)
	PreparePullRequest(context.Context, string, string) (*domain.PRDraft, error)
	SavePullRequest(context.Context, string, domain.PRDraft) (string, error)
}

func (a *App) SetPullRequestProvider(p PullRequestProvider) { a.pullRequests = p }
func (a *App) workflow() (WorkflowRepository, error) {
	r, ok := a.gitRunner.(WorkflowRepository)
	if !ok {
		return nil, fmt.Errorf("adaptador Git sem suporte ao fluxo integrado")
	}
	return r, nil
}
func (a *App) GetRepositoryLink(ctx context.Context, dir string) (*domain.RepositoryLink, error) {
	r, e := a.workflow()
	if e != nil {
		return nil, e
	}
	return r.RepositoryIdentity(ctx, dir)
}
func (a *App) CreateBranch(ctx context.Context, dir string, opts domain.BranchOptions) error {
	r, e := a.workflow()
	if e != nil {
		return e
	}
	return r.CreateBranch(ctx, dir, opts)
}
func (a *App) GetHistory(ctx context.Context, dir string, opts domain.HistoryOptions) (*domain.HistoryPage, error) {
	if opts.Skip < 0 || opts.Limit < 0 || opts.Limit > 500 {
		return nil, fmt.Errorf("histórico: offset >= 0 e limite entre 1 e 500")
	}
	if opts.Limit == 0 {
		opts.Limit = 100
	}
	r, e := a.workflow()
	if e != nil {
		return nil, e
	}
	return r.History(ctx, dir, opts)
}
func (a *App) GetCommitDetail(ctx context.Context, dir, hash string) (*domain.CommitDetail, error) {
	if len(hash) != 40 && len(hash) != 64 {
		return nil, fmt.Errorf("hash de commit inválido")
	}
	for _, c := range hash {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return nil, fmt.Errorf("hash de commit inválido")
		}
	}
	r, e := a.workflow()
	if e != nil {
		return nil, e
	}
	return r.CommitDetail(ctx, dir, hash)
}
func (a *App) prRepository(ctx context.Context, dir string) (*domain.RepositoryLink, error) {
	if a.pullRequests == nil {
		return nil, fmt.Errorf("PRs requerem gh instalado e autenticado (gh auth login)")
	}
	link, e := a.GetRepositoryLink(ctx, dir)
	if e != nil {
		return nil, e
	}
	if link.URL == "" {
		return nil, fmt.Errorf("selecione um remote GitHub não ambíguo")
	}
	u, e := url.Parse(link.URL)
	if e != nil || u.Host != "github.com" {
		return nil, fmt.Errorf("PRs nesta versão requerem um remote github.com")
	}
	return link, nil
}
func (a *App) ListPullRequests(ctx context.Context, dir string, o domain.PRListOptions) ([]domain.PullRequest, error) {
	if o.State == "" {
		o.State = "open"
	}
	if o.State != "all" && o.State != "open" && o.State != "closed" && o.State != "merged" {
		return nil, fmt.Errorf("estado de PR inválido")
	}
	if o.Limit == 0 {
		o.Limit = 100
	}
	if o.Limit < 1 || o.Limit > 500 {
		return nil, fmt.Errorf("limite de PRs deve estar entre 1 e 500")
	}
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	return a.pullRequests.List(ctx, dir, link.Slug, o)
}
func (a *App) ViewPullRequest(ctx context.Context, dir string, number int) (*domain.PullRequest, error) {
	if number <= 0 {
		return nil, fmt.Errorf("número do PR inválido")
	}
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	return a.pullRequests.View(ctx, dir, link.Slug, number)
}
func (a *App) PreparePullRequest(ctx context.Context, dir, base string) (*domain.PRDraft, error) {
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	r, e := a.workflow()
	if e != nil {
		return nil, e
	}
	head, detached, e := a.gitRunner.Branch(ctx, dir)
	if e != nil {
		return nil, e
	}
	if detached {
		return nil, fmt.Errorf("crie uma branch antes de abrir um PR")
	}
	if base == "" {
		base, e = r.DefaultBranch(ctx, dir)
		if e != nil {
			return nil, e
		}
		if base == "" {
			base, e = a.pullRequests.DefaultBranch(ctx, dir, link.Slug)
			if e != nil {
				return nil, e
			}
		}
	}
	if head == base {
		return nil, fmt.Errorf("a base precisa ser diferente da branch atual")
	}
	title, body, e := r.CommitDraft(ctx, dir, base)
	if e != nil {
		return nil, e
	}
	oid, e := a.gitRunner.RevParseHead(ctx, dir)
	if e != nil {
		return nil, e
	}
	return &domain.PRDraft{Repository: link.Slug, Head: head, HeadOID: oid, Base: base, Title: title, Body: body, Draft: true}, nil
}
func (a *App) SavePullRequest(ctx context.Context, dir string, d domain.PRDraft) (string, error) {
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return "", e
	}
	if d.Repository != link.Slug {
		return "", fmt.Errorf("destino mudou; revise o PR novamente")
	}
	if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Base) == "" {
		return "", fmt.Errorf("título e base são obrigatórios")
	}
	if d.Number < 0 {
		return "", fmt.Errorf("número do PR inválido")
	}
	if d.Number == 0 {
		branch, detached, e := a.gitRunner.Branch(ctx, dir)
		if e != nil {
			return "", e
		}
		oid, e := a.gitRunner.RevParseHead(ctx, dir)
		if e != nil {
			return "", e
		}
		if detached || branch != d.Head || oid != d.HeadOID {
			return "", fmt.Errorf("branch/HEAD mudou; prepare o PR novamente")
		}
		if d.Head == d.Base {
			return "", fmt.Errorf("base e head iguais")
		}
		r, e := a.workflow()
		if e != nil {
			return "", e
		}
		if e = r.PublishedHead(ctx, dir, link.Remote, d.Head, d.HeadOID); e != nil {
			return "", e
		}
	}
	return a.pullRequests.Save(ctx, dir, d)
}
