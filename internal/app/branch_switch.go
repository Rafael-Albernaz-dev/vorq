package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type BranchSwitchRepository interface {
	LocalBranches(context.Context, string) ([]domain.LocalBranch, error)
	SwitchLocalBranch(context.Context, string, string) error
}
type BranchSwitchService interface {
	ListLocalBranches(context.Context, string) ([]domain.LocalBranch, error)
	PrepareBranchSwitch(context.Context, string, string) (*domain.BranchSwitchPlan, error)
	SwitchBranch(context.Context, string, domain.BranchSwitchPlan) error
}

func (a *App) ListLocalBranches(ctx context.Context, dir string) ([]domain.LocalBranch, error) {
	repo, ok := a.gitRunner.(BranchSwitchRepository)
	if !ok {
		return nil, fmt.Errorf("adaptador sem suporte a troca de branch")
	}
	p, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return nil, err
	}
	return repo.LocalBranches(ctx, p.RootPath)
}
func (a *App) PrepareBranchSwitch(ctx context.Context, dir, name string) (*domain.BranchSwitchPlan, error) {
	p, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return nil, err
	}
	branches, err := a.ListLocalBranches(ctx, p.RootPath)
	if err != nil {
		return nil, err
	}
	var target *domain.LocalBranch
	for _, b := range branches {
		if b.Name == name {
			copy := b
			target = &copy
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("branch local %q não encontrada", name)
	}
	if target.Current {
		return nil, fmt.Errorf("a branch %q já está ativa", name)
	}
	if target.WorktreePath != "" && filepath.Clean(target.WorktreePath) != filepath.Clean(p.RootPath) {
		return nil, fmt.Errorf("branch %q em uso no worktree %q; use a aba Worktrees para acessá-la", name, target.WorktreePath)
	}
	from, _, err := a.gitRunner.Branch(ctx, p.RootPath)
	if err != nil {
		return nil, err
	}
	head, err := a.gitRunner.RevParseHead(ctx, p.RootPath)
	if err != nil {
		return nil, err
	}
	changes, err := a.gitRunner.ReadChanges(ctx, p.RootPath, from)
	if err != nil {
		return nil, err
	}
	if changes == nil || !changes.IsClean() {
		return nil, fmt.Errorf("troca bloqueada: workspace deve estar limpo; faça commit das alterações antes de trocar (stage e arquivos preservados)")
	}
	return &domain.BranchSwitchPlan{WorkspacePath: p.RootPath, From: from, FromHash: head, Target: target.Name, TargetHash: target.Hash}, nil
}
func (a *App) SwitchBranch(ctx context.Context, dir string, reviewed domain.BranchSwitchPlan) error {
	fresh, err := a.PrepareBranchSwitch(ctx, dir, reviewed.Target)
	if err != nil {
		return err
	}
	if *fresh != reviewed {
		return fmt.Errorf("origem, destino ou HEAD mudou; revise a troca de branch novamente")
	}
	repo := a.gitRunner.(BranchSwitchRepository)
	return repo.SwitchLocalBranch(ctx, fresh.WorkspacePath, fresh.Target)
}
