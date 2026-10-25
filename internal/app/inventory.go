package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

const InventoryPageSize = 200

type InventoryService interface {
	ReadInventoryDirectory(context.Context, string, string, int) ([]*domain.TreeNode, error)
	AbsoluteFilePath(context.Context, string, string) (string, error)
}

// ReadInventoryDirectory expands one directory only. Ignored data never enters
// ChangeSet and never becomes staged merely because it is visible.
func (a *App) ReadInventoryDirectory(ctx context.Context, dir, path string, offset int) ([]*domain.TreeNode, error) {
	if err := validateViewerPath(path); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("invalid page")
	}
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(proj.RootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("not a workspace directory")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	tracked, err := a.gitRunner.TrackedPaths(ctx, proj.RootPath)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, p := range tracked {
		known[p] = true
	}
	cs, err := a.gitRunner.ReadChanges(ctx, proj.RootPath, "")
	if err != nil {
		return nil, err
	}
	statuses := map[string]domain.FileStatus{}
	for _, s := range cs.Files {
		statuses[s.Path] = s
	}
	// Directory classification comes from descendants, not the directory path
	// itself (Git tracks file entries rather than ordinary directories).
	directories := map[string]string{}
	for p := range known {
		kind := "tracked"
		if statuses[p].IsUntracked() {
			kind = "untracked"
		}
		for parent := filepath.ToSlash(filepath.Dir(p)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if previous := directories[parent]; previous != "" && previous != kind {
				directories[parent] = "mixed"
			} else {
				directories[parent] = kind
			}
		}
	}
	var nodes []*domain.TreeNode
	for i := offset; i < len(entries) && i < offset+InventoryPageSize; i++ {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		entry := entries[i]
		if entry.Name() == ".git" {
			continue
		}
		p := filepath.ToSlash(filepath.Join(path, entry.Name()))
		tracking := "tracked"
		if entry.IsDir() && directories[p] != "" {
			tracking = directories[p]
		} else if statuses[p].IsUntracked() {
			tracking = "untracked"
		} else if !known[p] {
			ignored, e := a.gitRunner.IgnoredPath(ctx, proj.RootPath, p)
			if e != nil {
				return nil, e
			}
			if ignored {
				tracking = "ignored"
			} else {
				tracking = "untracked"
			}
		}
		nodes = append(nodes, &domain.TreeNode{Name: entry.Name(), Path: p, IsDir: entry.IsDir(), Lazy: entry.IsDir(), Tracking: tracking, Status: statuses[p]})
	}
	if offset+InventoryPageSize < len(entries) {
		nodes = append(nodes, &domain.TreeNode{Name: "Carregar mais…", Path: path, More: true, MoreOffset: offset + InventoryPageSize})
	}
	return nodes, nil
}

func (a *App) AbsoluteFilePath(ctx context.Context, dir, path string) (string, error) {
	if err := validateViewerPath(path); err != nil {
		return "", err
	}
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(proj.RootPath)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("select a file")
	}
	return filepath.Join(proj.RootPath, filepath.FromSlash(strings.TrimPrefix(path, "./"))), nil
}
