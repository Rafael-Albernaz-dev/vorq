package tui

// modalID names a dialog (VORQ-017): typed constants turn a misspelt
// dialog into a build error instead of a silently dead branch.
type modalID string

const (
	modalNone         modalID = ""
	modalAiError      modalID = "ai-error"
	modalAiModels     modalID = "ai-models"
	modalAiOptions    modalID = "ai-options"
	modalAiReview     modalID = "ai-review"
	modalAiSend       modalID = "ai-send"
	modalAppearance   modalID = "appearance"
	modalBranches     modalID = "branches"
	modalCommit       modalID = "commit"
	modalDiffReview   modalID = "diff-review"
	modalDiscard      modalID = "discard"
	modalError        modalID = "error"
	modalHelp         modalID = "help"
	modalPlan         modalID = "plan"
	modalPlanDiff     modalID = "plan-diff"
	modalPlanEdit     modalID = "plan-edit"
	modalPlanExecute  modalID = "plan-execute"
	modalPull         modalID = "pull"
	modalPush         modalID = "push"
	modalResult       modalID = "result"
	modalStageAdvice  modalID = "stage-advice"
	modalStageConfirm modalID = "stage-confirm"
	modalSuggest      modalID = "suggest"
	modalSwitchBranch modalID = "switch-branch"
	modalUnstage      modalID = "unstage"
	modalWorkflow     modalID = "workflow"
	modalWorktrees    modalID = "worktrees"
)
