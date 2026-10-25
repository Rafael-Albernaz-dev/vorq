package tui

import (
	"context"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// These states have one owner (the Bubble Tea update loop). Commands capture
// immutable values; embedding preserves pane APIs while separating concerns.
type operationsState struct {
	ctx                                  context.Context
	readCancel, fileCancel, dialogCancel context.CancelFunc
	loadID, fileID, dialogID             uint64
	loading, mutating, stale             bool
	busy, notice                         string
	lastRefresh                          time.Time
}
type navigationState struct {
	rightTab                                                  string
	rightActionIndex                                          int
	viewerGroup, activeTab, viewerTab, focusPane, lastDiffTab string
	studio                                                    *domain.WorkspaceTree
	snapshotSummary                                           domain.ChangeSummary
	rows                                                      []treeRow
	expanded                                                  map[string]bool
	positions                                                 map[string]treePosition
	selected, treeOffset, inventoryCount                      int
	viewer                                                    viewerState
}
type commitState struct {
	commitSummary, commitDescription, commitField, commitPhase string
	commitEditing                                              bool
	summaryCursor, descriptionCursor, descriptionOffset        int
	draftRevision                                              uint64
	reviewDiff, reviewMessage                                  string
	commitCount                                                int
	reviewPaths                                                []string
	commitType, partialApproved                                string
	suggested                                                  bool
}
type dialogState struct {
	localBranches                            []domain.LocalBranch
	branchIndex                              int
	branchSwitchPlan                         *domain.BranchSwitchPlan
	resultKind, resultBody, modal, lastError string
	modalScroll                              int
	discardPlan                              *domain.DiscardPlan
	pushTarget                               *domain.PushTarget
	pullTarget                               *domain.PullTarget
	worktrees                                []*domain.Workspace
	worktreeIndex                            int
}
type appearanceState struct {
	width, height, frame, themeIndex int
	isWide                           bool
	appearance                       *appearanceStore
	themeRevision                    uint64
	iconMode                         IconMode
}
type workspaceState struct {
	dir      string
	contexts map[string]workspaceContext
	drafts   map[string]commitDraft
}
