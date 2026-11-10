package tui

// aiTaskID names what an AI dialog is doing (VORQ-017).
type aiTaskID string

const (
	taskNone      aiTaskID = ""
	taskConfigure aiTaskID = "configure"
	taskPlan      aiTaskID = "plan"
	taskPr        aiTaskID = "pr"
	taskReview    aiTaskID = "review"
	taskStage     aiTaskID = "stage"
	taskSuggest   aiTaskID = "suggest"
)
