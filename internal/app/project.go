package app

import (
	"context"
	"fmt"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func (a *App) GetGitVersion(ctx context.Context) (string, error) {
	return a.gitRunner.Version(ctx)
}

func (a *App) ResolveProject(ctx context.Context, dir string) (*domain.Project, error) {
	root, common, err := a.gitRunner.ProjectPaths(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("not a git repository (or any parent up to mount point): %w", err)
	}
	return &domain.Project{ID: common, RootPath: root, GitDir: common, DefaultBaseBranch: "main"}, nil
}
