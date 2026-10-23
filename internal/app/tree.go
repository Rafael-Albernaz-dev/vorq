package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

const FilePreviewLimit = 256 * 1024

// GetWorkspaceTree lists tracked and non-ignored untracked paths through Git.
// This avoids traversing .git, ignored dependencies and external symlink targets.
func (a *App) GetWorkspaceTree(ctx context.Context, currentDir string) (*domain.WorkspaceTree, error) {
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	raw, err := a.gitRunner.Exec(ctx, proj.RootPath, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--renames")
	if err != nil {
		return nil, err
	}
	branch, branchErr := a.gitRunner.CurrentBranch(ctx, proj.RootPath)
	if branchErr != nil {
		branch = "(detached HEAD)"
	}
	changes, err := git.ParseStatus(raw, proj.RootPath, branch)
	if err != nil {
		return nil, err
	}
	changes.Ahead, changes.Behind, err = a.gitRunner.AheadBehind(ctx, proj.RootPath)
	if err != nil {
		return nil, err
	}
	listed, err := a.gitRunner.Exec(ctx, proj.RootPath, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	root := &domain.TreeNode{Name: filepath.Base(proj.RootPath), IsDir: true}
	nodes := map[string]*domain.TreeNode{"": root}
	insert := func(path string) *domain.TreeNode {
		parts := strings.Split(path, "/")
		parent := root
		prefix := ""
		for i, part := range parts {
			if part == "" || part == ".git" {
				return nil
			}
			if prefix != "" {
				prefix += "/"
			}
			prefix += part
			node := nodes[prefix]
			if node == nil {
				node = &domain.TreeNode{Name: part, Path: prefix, IsDir: i < len(parts)-1}
				nodes[prefix] = node
				parent.Children = append(parent.Children, node)
			}
			if i < len(parts)-1 {
				node.IsDir = true
			}
			parent = node
		}
		return parent
	}
	for _, path := range strings.Split(listed, "\x00") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if path != "" {
			insert(path)
		}
		if len(nodes) > 100000 {
			return nil, fmt.Errorf("workspace tree exceeds the 100000-node preview limit")
		}
	}
	for _, status := range changes.Files {
		if node := insert(status.Path); node != nil {
			node.Status = status
		}
	}
	for _, node := range nodes {
		sort.Slice(node.Children, func(i, j int) bool {
			a, b := node.Children[i], node.Children[j]
			if a.IsDir != b.IsDir {
				return a.IsDir
			}
			return a.Name < b.Name
		})
	}
	var aggregate func(*domain.TreeNode)
	aggregate = func(n *domain.TreeNode) {
		if n.Status.IsStaged() {
			n.StagedCount++
		}
		if n.Status.IsUnstaged() {
			n.UnstagedCount++
		}
		if n.Status.IsUntracked() {
			n.UntrackedCount++
		}
		if n.Status.IsDeleted() {
			n.DeletedCount++
		}
		for _, child := range n.Children {
			aggregate(child)
			n.StagedCount += child.StagedCount
			n.UnstagedCount += child.UnstagedCount
			n.UntrackedCount += child.UntrackedCount
			n.DeletedCount += child.DeletedCount
		}
	}
	aggregate(root)
	return &domain.WorkspaceTree{Workspace: domain.Workspace{Path: proj.RootPath, Branch: branch, IsDetached: branchErr != nil, Status: changes}, Root: root, Changes: changes}, nil
}

func validateViewerPath(path string) error {
	if err := validateCommitPaths([]string{path}); err != nil {
		return err
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if part == ".git" {
			return fmt.Errorf("Git metadata is not a workspace file")
		}
	}
	return nil
}

func (a *App) GetFileContent(ctx context.Context, currentDir, path string) (*domain.FileContent, error) {
	if err := validateViewerPath(path); err != nil {
		return nil, err
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
	info, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	result := &domain.FileContent{Path: path}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := root.Readlink(path)
		if err != nil {
			return nil, err
		}
		result.Notice = "Link simbólico → " + target + " (destino não aberto)"
		return result, nil
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %q", path)
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, FilePreviewLimit+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result.Truncated = len(data) > FilePreviewLimit
	if result.Truncated {
		data = data[:FilePreviewLimit]
	}
	// A size boundary may split the last UTF-8 rune; preserve complete runes.
	if result.Truncated && !utf8.Valid(data) {
		for n := 1; n < utf8.UTFMax && n < len(data); n++ {
			if utf8.Valid(data[:len(data)-n]) {
				data = data[:len(data)-n]
				break
			}
		}
	}
	result.Binary = bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data)
	if !result.Binary {
		result.Text = string(data)
	} else {
		result.Notice = "Arquivo binário — preview desativado"
	}
	return result, nil
}

func (a *App) GetFileDiff(ctx context.Context, currentDir, path string) (*domain.FileDiff, error) {
	if err := validateViewerPath(path); err != nil {
		return nil, err
	}
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return nil, err
	}
	staged, st, err := a.gitRunner.FileDiff(ctx, proj.RootPath, path, true)
	if err != nil {
		return nil, err
	}
	unstaged, un, err := a.gitRunner.FileDiff(ctx, proj.RootPath, path, false)
	if err != nil {
		return nil, err
	}
	return &domain.FileDiff{Staged: staged, Unstaged: unstaged, Truncated: st || un}, nil
}

// ToggleStage keeps staging rules shared across UIs. Partially staged files are
// unstaged as a whole, preserving every byte of the working copy.
func (a *App) ToggleStage(ctx context.Context, currentDir, path string) error {
	if err := validateViewerPath(path); err != nil {
		return err
	}
	proj, err := a.ResolveProject(ctx, currentDir)
	if err != nil {
		return err
	}
	raw, err := a.gitRunner.Exec(ctx, proj.RootPath, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--renames")
	if err != nil {
		return err
	}
	changes, err := git.ParseStatus(raw, proj.RootPath, "")
	if err != nil {
		return err
	}
	for _, f := range changes.Files {
		if f.Path == path {
			paths := []string{"./" + path}
			if f.OrigPath != "" {
				paths = append(paths, "./"+f.OrigPath)
			}
			if f.IsStaged() {
				return a.UnstageFiles(ctx, proj.RootPath, paths)
			}
			return a.StageFiles(ctx, proj.RootPath, paths)
		}
	}
	return fmt.Errorf("arquivo sem alterações para stage: %q", path)
}
