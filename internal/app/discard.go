package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

// PrepareDiscard never mutates. It accepts one changed regular file, never a
// directory/pathspec, metadata, ignored file, symlink, rename or merge conflict.
func (a *App) PrepareDiscard(ctx context.Context, currentDir, path string) (*domain.DiscardPlan, error) {
	if err := validateViewerPath(path); err != nil {
		return nil, err
	}
	if path == "." || filepath.Clean(path) != path || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("descarte exige o caminho exato de um arquivo")
	}
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(proj.RootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	// Even internal symlinks can alias another reviewed path. Do not follow them.
	parts := strings.Split(path, string(filepath.Separator))
	for i := range parts {
		info, statErr := root.Lstat(filepath.Join(parts[:i+1]...))
		if os.IsNotExist(statErr) && i == len(parts)-1 {
			break
		}
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return nil, fmt.Errorf("descarte bloqueado para links, diretórios e submódulos: %q", path)
		}
	}
	ignored, err := a.gitRunner.Exec(ctx, proj.RootPath, "--literal-pathspecs", "ls-files", "--cached", "--others", "--ignored", "--exclude-standard", "-z", "--", path)
	if err != nil {
		return nil, err
	}
	if ignored != "" {
		return nil, fmt.Errorf("arquivo ignorado protegido: %q", path)
	}
	raw, err := a.gitRunner.Exec(ctx, proj.RootPath, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--renames")
	if err != nil {
		return nil, err
	}
	cs, err := git.ParseStatus(raw, proj.RootPath, "")
	if err != nil {
		return nil, err
	}
	var status *domain.FileStatus
	for _, f := range cs.Files {
		if f.Path == path {
			copy := f
			status = &copy
			break
		}
	}
	if status == nil {
		return nil, fmt.Errorf("arquivo sem alterações para descartar: %q", path)
	}
	if status.IsConflicted() || status.OrigPath != "" || strings.ContainsAny(status.Staged+status.Unstaged, "RCT") {
		return nil, fmt.Errorf("descarte bloqueado para renames, conflitos e mudanças de tipo: %q", path)
	}
	index, err := a.gitRunner.Exec(ctx, proj.RootPath, "--literal-pathspecs", "ls-files", "--stage", "-z", "--", path)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(index, "160000 ") || strings.HasPrefix(index, "120000 ") {
		return nil, fmt.Errorf("descarte de submódulo ou link simbólico bloqueado")
	}
	plan := &domain.DiscardPlan{WorkspacePath: proj.RootPath, Path: path, Delete: status.IsUntracked() || status.Staged == "A", Staged: status.IsStaged()}
	plan.Impact = "Todas as modificações fora do stage serão revertidas para o índice."
	if plan.Staged {
		plan.Impact = "Todas as modificações deste arquivo, no stage e fora dele, serão revertidas para HEAD."
	}
	if plan.Delete {
		plan.Impact = "Este arquivo novo será excluído permanentemente do disco; sua preparação no stage também será removida."
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "%q\x00%q\x00%q\x00", *status, index, plan.WorkspacePath)
	// HEAD participates in review identity for tracked files; unborn new files
	// require no HEAD and can still be discarded safely after confirmation.
	if !plan.Delete {
		head, err := a.gitRunner.RevParseHead(ctx, proj.RootPath)
		if err != nil {
			return nil, err
		}
		entry, err := a.gitRunner.Exec(ctx, proj.RootPath, "--literal-pathspecs", "ls-tree", "--full-tree", "-z", head, "--", path)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(entry, "160000 ") || strings.HasPrefix(entry, "120000 ") {
			return nil, fmt.Errorf("descarte de submódulo ou link simbólico bloqueado, inclusive quando excluído")
		}
		fmt.Fprint(hash, head)
	}
	f, err := root.Open(path)
	if err == nil {
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(hash, "\x00%v\x00", info.Mode())
		buf := make([]byte, 32*1024)
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			n, readErr := f.Read(buf)
			if n > 0 {
				_, _ = hash.Write(buf[:n])
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return nil, readErr
			}
		}
	} else if os.IsNotExist(err) {
		fmt.Fprint(hash, "\x00missing")
	} else {
		return nil, err
	}
	plan.Fingerprint = fmt.Sprintf("%x", hash.Sum(nil))
	return plan, ctx.Err()
}

// DiscardFile requires both explicit confirmation and the exact reviewed plan.
// Validation failures leave the index and working file untouched.
func (a *App) DiscardFile(ctx context.Context, currentDir string, plan domain.DiscardPlan, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("descarte exige confirmação explícita")
	}
	actual, err := a.PrepareDiscard(ctx, currentDir, plan.Path)
	if err != nil {
		return err
	}
	if *actual != plan {
		return fmt.Errorf("arquivo, índice ou HEAD mudou após a revisão; confirme um novo descarte")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Delete {
		if plan.Staged {
			if _, err := a.gitRunner.Exec(ctx, plan.WorkspacePath, "--literal-pathspecs", "rm", "--cached", "-f", "--", plan.Path); err != nil {
				return err
			}
		}
		root, err := os.OpenRoot(plan.WorkspacePath)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := root.Remove(plan.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stage removido, mas exclusão do arquivo falhou: %w", err)
		}
		return nil
	}
	args := []string{"--literal-pathspecs", "restore", "--worktree"}
	if plan.Staged {
		args = append(args, "--source=HEAD", "--staged")
	}
	_, err = a.gitRunner.Exec(ctx, plan.WorkspacePath, append(args, "--", plan.Path)...)
	return err
}
