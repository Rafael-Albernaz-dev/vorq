package git

import (
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// ParseWorktreeList parses the NUL-delimited output of `git worktree list --porcelain -z`.
func ParseWorktreeList(output string) ([]*domain.Workspace, error) {
	if len(output) == 0 {
		return []*domain.Workspace{}, nil
	}

	// Split by NUL byte
	tokens := strings.Split(output, "\x00")
	var worktrees []*domain.Workspace
	var current *domain.Workspace

	finishCurrent := func() {
		if current != nil && current.Path != "" {
			if current.Branch == "" {
				if current.IsDetached {
					current.Branch = "(detached HEAD)"
				} else if current.IsBare {
					current.Branch = "(bare)"
				} else {
					current.Branch = "(unknown)"
				}
			}
			worktrees = append(worktrees, current)
			current = nil
		}
	}

	for _, token := range tokens {
		if token == "" {
			// An empty token between records marks the boundary of a worktree block.
			finishCurrent()
			continue
		}

		if strings.HasPrefix(token, "worktree ") {
			finishCurrent()
			current = &domain.Workspace{
				Path: strings.TrimPrefix(token, "worktree "),
			}
			continue
		}

		if current == nil {
			continue
		}

		switch {
		case strings.HasPrefix(token, "HEAD "):
			current.CommitHash = strings.TrimPrefix(token, "HEAD ")
		case strings.HasPrefix(token, "branch "):
			ref := strings.TrimPrefix(token, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case token == "bare":
			current.IsBare = true
		case token == "detached":
			current.IsDetached = true
		case strings.HasPrefix(token, "locked"):
			current.IsLocked = true
			if len(token) > len("locked ") {
				current.LockReason = strings.TrimPrefix(token, "locked ")
			}
		case strings.HasPrefix(token, "prunable"):
			current.IsPrunable = true
			if len(token) > len("prunable ") {
				current.PruneReason = strings.TrimPrefix(token, "prunable ")
			}
		}
	}

	// Flush any trailing record
	finishCurrent()

	// The first worktree listed in `git worktree list` is always the main worktree.
	if len(worktrees) > 0 {
		worktrees[0].IsMain = true
	}

	return worktrees, nil
}
