package tui

import (
	"context"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// These states have one owner (the Bubble Tea update loop). Commands capture
// immutable values; embedding preserves pane APIs while separating concerns.
type operationsState struct {
	ctx                                  context.Context
	readCancel, fileCancel, dialogCancel context.CancelFunc
	loadID, fileID, dialogID             uint64
	loading, mutating, stale             bool
	reloadInventory, explicitReload      bool
	busy, notice                         string
}
type navigationState struct {
	rightTab                                       string
	rightActionIndex                               int
	viewerGroup, viewerTab, focusPane, lastDiffTab string
	activeTab                                      treeTab
	studio                                         *domain.WorkspaceTree
	snapshotSummary                                domain.ChangeSummary
	rows                                           []treeRow
	expanded                                       map[string]bool
	positions                                      map[treeTab]treePosition
	pendingSelection                               treePosition
	fullscreen, viewerWrap                         bool
	fullscreenPath                                 string
	selected, treeOffset, inventoryCount           int
	viewer                                         viewerState
}
type commitState struct {
	aiTask                                                     aiTaskID
	commitPlan                                                 *domain.CommitPlan
	diffReview                                                 *domain.DiffReview
	stageAdvice                                                *domain.StageAdvice
	planGroup                                                  int
	planEditKind, planEditValue                                string
	aiProvider, aiError                                        string
	aiModel, suggestionSource                                  string
	aiModels                                                   []domain.AIModel
	aiModelIndex                                               int
	aiThenReview                                               bool
	aiProposal                                                 *domain.AIProposal
	aiRevision                                                 uint64
	commitSummary, commitDescription, commitField, commitPhase string
	commitEditing                                              bool
	summaryCursor, descriptionCursor, descriptionOffset        int
	draftRevision                                              uint64
	reviewDiff, reviewMessage                                  string
	commitCount                                                int
	reviewPaths                                                []string
	reviewGuard                                                *domain.GuardReport
	commitType, partialApproved                                string
	suggested                                                  bool
}
type dialogState struct {
	localBranches                     []domain.LocalBranch
	branchIndex                       int
	branchSwitchPlan                  *domain.BranchSwitchPlan
	resultKind, resultBody, lastError string
	modal                             modalID
	terminalRetryKind                 string
	lastPRDraft                       *domain.PRDraft
	modalScroll                       int
	discardPlan                       *domain.DiscardPlan
	pushTarget                        *domain.PushTarget
	guardFor                          modalID // commit or push awaiting a typed override
	guardInput                        string
	guardOverride                     bool // CONFIRMAR typed for the reviewed operation
	pullTarget                        *domain.PullTarget
	syncTarget                        *domain.SyncTarget
	worktrees                         []*domain.Workspace
	worktreeIndex                     int
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
