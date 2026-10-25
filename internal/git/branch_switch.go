package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func (r *Repository) LocalBranches(ctx context.Context, dir string) ([]domain.LocalBranch, error) {
	raw, err := r.Exec(ctx, dir, "for-each-ref", "--sort=refname", "--format=%(refname:lstrip=2)%00%(objectname)%00%(HEAD)%00%(worktreepath)%00", "refs/heads/")
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	fields := strings.Split(raw, "\x00")
	if len(fields)%4 != 1 || fields[len(fields)-1] != "\n" {
		return nil, fmt.Errorf("lista de branches inválida ou incompleta")
	}
	branches := make([]domain.LocalBranch, 0, len(fields)/4)
	for i := 0; i+3 < len(fields); i += 4 {
		name := strings.TrimPrefix(fields[i], "\n")
		if name == "" || fields[i+1] == "" {
			return nil, fmt.Errorf("metadados de branch inválidos")
		}
		branches = append(branches, domain.LocalBranch{Name: name, Hash: fields[i+1], Current: fields[i+2] == "*", WorktreePath: fields[i+3]})
	}
	return branches, nil
}
func (r *Repository) SwitchLocalBranch(ctx context.Context, dir, name string) error {
	_, err := r.Exec(ctx, dir, "switch", "--no-guess", "--no-overwrite-ignore", "--no-recurse-submodules", "--", name)
	return err
}
