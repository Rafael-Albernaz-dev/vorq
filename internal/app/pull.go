package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// PullRepository exposes a pinned fetch and an explicit fast-forward. Splitting
// these steps lets the core revalidate HEAD/status after the network operation.
type PullRepository interface {
	PullSourceID(context.Context, string, string) (string, error)
	FetchPull(context.Context, string, string, string) (string, string, error)
	ApplyPull(context.Context, string, string) (string, error)
	DropPullRef(context.Context, string, string, string) error
}
type PullService interface {
	GetPullTarget(context.Context, string) (*domain.PullTarget, error)
	PullChanges(context.Context, string, domain.PullTarget) (*domain.PullResult, error)
}

func (a *App) GetPullTarget(ctx context.Context, dir string) (*domain.PullTarget, error) {
	repo, ok := a.gitRunner.(PullRepository)
	if !ok {
		return nil, fmt.Errorf("adaptador sem suporte a pull")
	}
	p, e := a.ResolveProject(ctx, dir)
	if e != nil {
		return nil, e
	}
	branch, e := a.gitRunner.CurrentBranch(ctx, p.RootPath)
	if e != nil {
		return nil, fmt.Errorf("atualização requer uma branch local ativa (HEAD destacado não é aceito)")
	}
	head, e := a.gitRunner.RevParseHead(ctx, p.RootPath)
	if e != nil {
		return nil, e
	}
	remote, ref, e := a.gitRunner.UpstreamTarget(ctx, p.RootPath, branch)
	if e != nil {
		return nil, e
	}
	if remote == "" || remote == "." || !strings.HasPrefix(ref, "refs/heads/") {
		return nil, fmt.Errorf("configure o upstream da branch antes de atualizar (use o Push explícito para uma branch nova)")
	}
	changes, e := a.gitRunner.ReadChanges(ctx, p.RootPath, branch)
	if e != nil {
		return nil, e
	}
	if changes == nil || !changes.IsClean() {
		return nil, fmt.Errorf("atualização bloqueada: preserve suas alterações com commit antes de atualizar; stage e arquivos não foram alterados")
	}
	source, e := repo.PullSourceID(ctx, p.RootPath, remote)
	if e != nil {
		return nil, fmt.Errorf("não foi possível identificar a origem do pull")
	}
	ahead, behind, e := a.gitRunner.AheadBehind(ctx, p.RootPath)
	if e != nil {
		return nil, e
	}
	return &domain.PullTarget{WorkspacePath: p.RootPath, Branch: branch, Remote: remote, RemoteBranch: strings.TrimPrefix(ref, "refs/heads/"), HeadHash: head, SourceID: source, Ahead: ahead, Behind: behind}, nil
}
func samePullTarget(a, b domain.PullTarget) bool {
	return a.WorkspacePath == b.WorkspacePath && a.Branch == b.Branch && a.Remote == b.Remote && a.RemoteBranch == b.RemoteBranch && a.HeadHash == b.HeadHash && a.SourceID == b.SourceID
}
func (a *App) PullChanges(ctx context.Context, dir string, reviewed domain.PullTarget) (result *domain.PullResult, err error) {
	target, e := a.GetPullTarget(ctx, dir)
	if e != nil {
		return nil, e
	}
	if !samePullTarget(*target, reviewed) {
		return nil, fmt.Errorf("origem, branch ou HEAD mudou; revise a atualização novamente")
	}
	repo := a.gitRunner.(PullRepository)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	hash, ref, e := repo.FetchPull(ctx, target.WorkspacePath, target.Remote, target.RemoteBranch)
	if ref != "" {
		defer func() {
			cleanCtx, cleanCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cleanCancel()
			if cleanErr := repo.DropPullRef(cleanCtx, target.WorkspacePath, ref, hash); cleanErr != nil {
				if result != nil {
					result.Output += "\nAviso: referência temporária da atualização foi preservada."
				} else {
					err = errors.Join(err, fmt.Errorf("limpeza da referência temporária: %w", cleanErr))
				}
			}
		}()
	}
	if e != nil {
		return nil, fmt.Errorf("busca não concluída; HEAD preservado, referências remotas podem ter sido atualizadas: %w", e)
	}
	fresh, e := a.GetPullTarget(ctx, dir)
	if e != nil {
		return nil, fmt.Errorf("busca concluída, aplicação bloqueada: %w", e)
	}
	if !samePullTarget(*fresh, reviewed) {
		return nil, fmt.Errorf("busca concluída, mas origem/branch/HEAD mudou; nenhuma integração solicitada")
	}
	out, e := repo.ApplyPull(ctx, target.WorkspacePath, hash)
	if e != nil {
		return nil, fmt.Errorf("fast-forward não confirmado; não repita sem conferir o estado (a busca já ocorreu): %w", e)
	}
	return &domain.PullResult{Target: *target, Output: out}, nil
}
