package tui

import (
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type treeMsg struct {
	id   uint64
	tree *domain.WorkspaceTree
	err  error
}
type fileMsg struct {
	id       uint64
	path     string
	content  *domain.FileContent
	diff     *domain.FileDiff
	err      error
	prepared *viewerState
}
type proposalMsg struct {
	id, revision uint64
	proposal     *domain.CommitProposal
	err          error
}
type reviewMsg struct {
	id, revision uint64
	changes      *domain.ChangeSet
	err          error
}
type pushTargetMsg struct {
	id     uint64
	target *domain.PushTarget
	err    error
}
type worktreesMsg struct {
	id        uint64
	worktrees []*domain.Workspace
	err       error
}
type actionMsg struct {
	id           uint64
	kind, notice string
	created      bool
	err          error
}
type workspaceMsg struct {
	id   uint64
	tree *domain.WorkspaceTree
	err  error
}
type tickMsg time.Time
type commitDraft struct{ summary, description string }
type workspaceContext struct {
	draft                                                     commitDraft
	activeTab, viewerTab, focusPane, lastDiffTab, viewerGroup string
	expanded                                                  map[string]bool
	positions                                                 map[string]treePosition
	selected, treeOffset                                      int
	viewer                                                    viewerState
}
