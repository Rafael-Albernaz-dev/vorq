package app

import (
	"context"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// Repository is a consumer-owned port. Native command construction, output
// formats and parsers belong to its adapter; use cases cannot issue raw Exec.
type Repository interface {
	ProjectRepository
	WorkspaceRepository
	CommitRepository
	ExplorerRepository
	PushRepository
}
type ProjectRepository interface {
	Version(context.Context) (string, error)
	ProjectPaths(context.Context, string) (string, string, error)
	CurrentBranch(context.Context, string) (string, error)
	Branch(context.Context, string) (string, bool, error)
	RevParseHead(context.Context, string) (string, error)
	AheadBehind(context.Context, string) (int, int, error)
}
type WorkspaceRepository interface {
	ReadWorktrees(context.Context, string) ([]*domain.Workspace, error)
	ReadChanges(context.Context, string, string) (*domain.ChangeSet, error)
	IgnoredFiles(context.Context, string) ([]string, error)
	BranchExists(context.Context, string, string) (bool, error)
	WorktreeAdd(context.Context, string, string, string, string, bool) error
	WorktreeRemove(context.Context, string, string, bool) error
	WorktreePrune(context.Context, string) (string, error)
	BranchDelete(context.Context, string, string, bool) error
}
type CommitRepository interface {
	Add(context.Context, string, ...string) error
	Reset(context.Context, string, ...string) error
	Commit(context.Context, string, string) (string, error)
	DiffCached(context.Context, string) (string, error)
	Diff(context.Context, string) (string, error)
	PreviewBase(context.Context, string) (string, error)
	WorkingTreeChanges(context.Context, string, string) ([]domain.FileStatus, error)
	TreeEntry(context.Context, string, string, string) (*domain.GitEntry, error)
	Blob(context.Context, string, string) (string, error)
	FileMode(context.Context, string) (bool, error)
	HashFile(context.Context, string, string) (string, error)
	ReadCommitInfo(context.Context, string, string) (*domain.CommitInfo, error)
	CommitFiles(context.Context, string, string, []string) ([]string, error)
}
type ExplorerRepository interface {
	TrackedPaths(context.Context, string) ([]string, error)
	FileDiff(context.Context, string, string, bool) (string, bool, error)
	IgnoredPath(context.Context, string, string) (bool, error)
	IndexEntry(context.Context, string, string) (*domain.GitEntry, error)
	RemoveIndexEntry(context.Context, string, string) error
	RestoreFile(context.Context, string, string, bool) error
}
type PushRepository interface {
	Remotes(context.Context, string) ([]string, error)
	Upstream(context.Context, string) (string, error)
	UpstreamTarget(context.Context, string, string) (string, string, error)
	PushDestinationID(context.Context, string, string) (string, error)
	Push(context.Context, string, string, string, string, bool) (string, error)
}
