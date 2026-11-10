package app

import "github.com/Rafael-Albernaz-dev/vorq/internal/domain"

// SpawnOptions holds parameters for provisioning a new worktree.
type SpawnOptions struct {
	BranchName       string
	BaseBranch       string
	TargetDir        string
	CheckoutExisting bool
	CopyFiles        []string
}

// SpawnResult holds the outcome of a spawn operation.
type SpawnResult struct {
	Workspace      *domain.Workspace
	CopiedFiles    []string
	IsReusedBranch bool
	BaseBranch     string // the base actually used (resolved when not given)
}

// App implements the Service interface coordinating adapters and business logic.
type App struct {
	gitRunner    Repository
	pullRequests PullRequestProvider
	ai           AIProvider
}

// NewApp initializes the application core with required adapters.
func NewApp(gitRunner Repository) *App {
	return &App{
		gitRunner: gitRunner,
	}
}
