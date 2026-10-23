package domain

import "time"

// Project represents the Git repository identity and its resolved settings.
type Project struct {
	ID                string `json:"id"`        // Unique project ID (typically canonical git-common-dir path)
	RootPath          string `json:"root_path"` // Path to the main repository
	GitDir            string `json:"git_dir"`   // Resolved git common dir
	DefaultBaseBranch string `json:"default_base_branch"`
}

// Workspace represents an isolated Git worktree or the main working tree.
type Workspace struct {
	Path        string     `json:"path"`
	Branch      string     `json:"branch"`      // e.g. "main", "feat/x" or "(detached HEAD)"
	CommitHash  string     `json:"commit_hash"` // 40-character commit hash or short
	IsMain      bool       `json:"is_main"`     // true if this is the root working tree
	IsCurrent   bool       `json:"is_current"`  // true if this workspace matches current working directory
	IsBare      bool       `json:"is_bare"`     // true if bare repository
	IsDetached  bool       `json:"is_detached"` // true if in detached HEAD state
	IsLocked    bool       `json:"is_locked"`   // true if locked by git worktree lock
	LockReason  string     `json:"lock_reason,omitempty"`
	IsPrunable  bool       `json:"is_prunable"` // true if marked as prunable by Git
	PruneReason string     `json:"prune_reason,omitempty"`
	Status      *ChangeSet `json:"status,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
}

// FileStatus represents the status of a specific file in Git.
type FileStatus struct {
	Path     string `json:"path"`
	OrigPath string `json:"orig_path,omitempty"` // For renames/copies
	Staged   string `json:"staged"`              // 'M', 'A', 'D', 'R', ' '
	Unstaged string `json:"unstaged"`            // 'M', 'D', '?', ' '
}

// ChangeSet holds current working tree and index changes.
type ChangeSet struct {
	WorkspacePath string       `json:"workspace_path"`
	Branch        string       `json:"branch"`
	Ahead         int          `json:"ahead"`
	Behind        int          `json:"behind"`
	Files         []FileStatus `json:"files"`
	StagedDiff    string       `json:"staged_diff,omitempty"`
	UnstagedDiff  string       `json:"unstaged_diff,omitempty"`
}

// IsClean returns true if there are no staged, unstaged or untracked modifications.
func (c *ChangeSet) IsClean() bool {
	return len(c.Files) == 0
}

// StagedCount returns the number of files with staged changes.
func (c *ChangeSet) StagedCount() int {
	count := 0
	for _, f := range c.Files {
		if f.IsStaged() {
			count++
		}
	}
	return count
}

// UnstagedCount returns the number of files with unstaged changes.
func (c *ChangeSet) UnstagedCount() int {
	count := 0
	for _, f := range c.Files {
		if f.Unstaged != "" && f.Unstaged != " " && f.Unstaged != "?" {
			count++
		}
	}
	return count
}

// UntrackedCount returns the number of untracked files.
func (c *ChangeSet) UntrackedCount() int {
	count := 0
	for _, f := range c.Files {
		if f.Staged == "?" || f.Unstaged == "?" {
			count++
		}
	}
	return count
}

// SuggestionEngineType identifies the mechanism that generated the proposal.
type SuggestionEngineType string

const (
	EngineDeterministicRules SuggestionEngineType = "rules"
	EngineLocalModel         SuggestionEngineType = "local_model"
	EngineManualInput        SuggestionEngineType = "manual"
)

// CommitProposal represents a suggested commit message and its metadata.
type CommitProposal struct {
	Subject      string               `json:"subject"`
	Body         string               `json:"body,omitempty"`
	Engine       SuggestionEngineType `json:"engine"`
	ModelName    string               `json:"model_name,omitempty"`
	GeneratedAt  time.Time            `json:"generated_at"`
	IsUserEdited bool                 `json:"is_user_edited"`
}

// CommitResult details a successfully created local commit.
type CommitResult struct {
	Hash           string   `json:"hash"`
	ShortHash      string   `json:"short_hash"`
	Subject        string   `json:"subject"`
	FilesCommitted []string `json:"files_committed"`
}

// PushTarget is a preview of one explicit branch publication. DestinationID
// fingerprints push URLs without exposing embedded credentials in the UI.
type PushTarget struct {
	WorkspacePath string `json:"workspace_path"`
	Branch        string `json:"branch"`
	Remote        string `json:"remote"`
	RemoteBranch  string `json:"remote_branch"`
	HasUpstream   bool   `json:"has_upstream"`
	Ahead         int    `json:"ahead"` // -1 when the remote-tracking ref is unavailable
	Behind        int    `json:"behind"`
	HeadHash      string `json:"head_hash"`
	DestinationID string `json:"destination_id"`
}

// PushOptions must carry the target actually reviewed by the caller.
type PushOptions struct {
	Target PushTarget `json:"target"`
}

type PushResult struct {
	Target PushTarget `json:"target"`
	Output string     `json:"output"`
}

func (f FileStatus) IsStaged() bool {
	return !f.IsConflicted() && f.Staged != "" && f.Staged != " " && f.Staged != "?"
}
func (f FileStatus) IsConflicted() bool {
	return f.Staged == "U" || f.Unstaged == "U" || (f.Staged == "A" && f.Unstaged == "A") || (f.Staged == "D" && f.Unstaged == "D")
}
func (f FileStatus) IsUntracked() bool { return f.Staged == "?" || f.Unstaged == "?" }
func (f FileStatus) IsUnstaged() bool {
	return f.Unstaged != "" && f.Unstaged != " " && f.Unstaged != "?"
}
func (f FileStatus) IsDeleted() bool { return f.Staged == "D" || f.Unstaged == "D" }

// TreeNode paths are always relative to the current workspace root.
type TreeNode struct {
	Name           string      `json:"name"`
	Path           string      `json:"path"`
	IsDir          bool        `json:"is_dir"`
	Status         FileStatus  `json:"status"`
	Children       []*TreeNode `json:"children,omitempty"`
	StagedCount    int         `json:"staged_count"`
	UnstagedCount  int         `json:"unstaged_count"`
	UntrackedCount int         `json:"untracked_count"`
	DeletedCount   int         `json:"deleted_count"`
}

type WorkspaceTree struct {
	Workspace Workspace  `json:"workspace"`
	Root      *TreeNode  `json:"root"`
	Changes   *ChangeSet `json:"changes"`
}

type FileContent struct {
	Path      string `json:"path"`
	Text      string `json:"text"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	Notice    string `json:"notice,omitempty"`
}

type FileDiff struct {
	Staged    string `json:"staged"`
	Unstaged  string `json:"unstaged"`
	Truncated bool   `json:"truncated"`
}

// Message returns the formatted commit message.
func (p *CommitProposal) Message() string {
	if p.Body == "" {
		return p.Subject
	}
	return p.Subject + "\n\n" + p.Body
}

// SafetyViolationType enumerates reasons why an operation might be blocked.
type SafetyViolationType string

const (
	ViolationDirtyWorkingTree SafetyViolationType = "dirty_working_tree"
	ViolationUntrackedFiles   SafetyViolationType = "untracked_files"
	ViolationIgnoredFiles     SafetyViolationType = "ignored_sensitive_files"
	ViolationMainWorkspace    SafetyViolationType = "cannot_remove_main_workspace"
	ViolationActiveWorkspace  SafetyViolationType = "cannot_remove_active_workspace"
	ViolationBranchConflict   SafetyViolationType = "branch_conflict"
	ViolationPathConflict     SafetyViolationType = "path_conflict"
)

// SafetyViolation details a specific blocked condition.
type SafetyViolation struct {
	Type        SafetyViolationType `json:"type"`
	Description string              `json:"description"`
	ItemPath    string              `json:"item_path,omitempty"`
}

// SafetyReport determines whether an operation is allowed or blocked.
type SafetyReport struct {
	IsSafe     bool              `json:"is_safe"`
	Violations []SafetyViolation `json:"violations"`
}
