package app

import (
	"context"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type ProjectService interface {
	GetGitVersion(context.Context) (string, error)
	ResolveProject(context.Context, string) (*domain.Project, error)
}
type WorkspaceReader interface {
	ListWorktrees(context.Context, string) ([]*domain.Workspace, error)
	GetWorkspaceStatus(context.Context, string, string) (*domain.Workspace, *domain.ChangeSet, error)
}
type WorkspaceService interface {
	SpawnWorkspace(context.Context, string, SpawnOptions) (*SpawnResult, error)
	AuditWorkspaceSafety(context.Context, string, *domain.Workspace) (*domain.SafetyReport, error)
	RemoveWorkspace(context.Context, string, string, bool) (*domain.SafetyReport, error)
	PruneWorkspaces(context.Context, string) (string, error)
}
type StageService interface {
	StageFiles(context.Context, string, []string) error
	UnstageFiles(context.Context, string, []string) error
	ToggleStage(context.Context, string, string) error
}
type CommitService interface {
	GetCommitChanges(context.Context, string) (*domain.ChangeSet, error)
	ProposeCommit(context.Context, string, string) (*domain.CommitProposal, error)
	PreviewCommit(context.Context, string, string, bool) (*domain.ChangeSet, *domain.CommitProposal, error)
	CreateCommit(context.Context, string, string) (*domain.CommitResult, error)
	CreateReviewedCommit(context.Context, string, string, string) (*domain.CommitResult, error)
}
type PushService interface {
	GetPushTarget(context.Context, string) (*domain.PushTarget, error)
	PushChanges(context.Context, string, domain.PushOptions) (*domain.PushResult, error)
}
type ExplorerService interface {
	GetWorkspaceTree(context.Context, string) (*domain.WorkspaceTree, error)
	GetFileContent(context.Context, string, string) (*domain.FileContent, error)
	GetFileDiff(context.Context, string, string) (*domain.FileDiff, error)
}
type DiscardService interface {
	PrepareDiscard(context.Context, string, string) (*domain.DiscardPlan, error)
	DiscardFile(context.Context, string, domain.DiscardPlan, bool) error
}

// Service remains the full application contract for composition and clients
// needing every workflow. Consumers depend on the smaller contracts above.
type Service interface {
	ProjectService
	WorkspaceReader
	WorkspaceService
	StageService
	CommitService
	PushService
	ExplorerService
	DiscardService
}

// StudioService excludes provisioning, teardown, CLI preview and direct commit.
type StudioService interface {
	StageService
	PushService
	ExplorerService
	DiscardService
	ListWorktrees(context.Context, string) ([]*domain.Workspace, error)
	GetCommitChanges(context.Context, string) (*domain.ChangeSet, error)
	ProposeCommit(context.Context, string, string) (*domain.CommitProposal, error)
	CreateReviewedCommit(context.Context, string, string, string) (*domain.CommitResult, error)
}
