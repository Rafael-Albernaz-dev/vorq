package domain

type PullTarget struct {
	WorkspacePath, Branch, Remote, RemoteBranch, HeadHash, SourceID string
	Ahead, Behind                                                   int
}
type PullResult struct {
	Target PullTarget
	Output string
}

// RepositoryLink is a credential-free web identity of the selected remote.
type RepositoryLink struct{ Remote, URL, Slug string }
type BranchOptions struct {
	Name, Base string
	Stay       bool
}
type HistoryOptions struct {
	Skip, Limit int
	Ref, Search string
}
type HistoryLine struct{ Graph, Hash, Parents, Author, Date, Refs, Subject string }
type HistoryPage struct {
	Lines            []HistoryLine
	HasMore, Shallow bool
	Snapshot         string
	Skip, Count      int
}
type CommitDetail struct{ Hash, Text string }
type PullRequest struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	URL            string `json:"url"`
	State          string `json:"state"`
	Head           string `json:"headRefName"`
	Base           string `json:"baseRefName"`
	HeadOID        string `json:"headRefOid"`
	Draft          bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	MergeCommit    *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Comments []struct {
		Body   string `json:"body"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
	} `json:"comments"`
	Reviews []struct {
		State  string `json:"state"`
		Body   string `json:"body"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
	} `json:"reviews"`
	Checks []struct {
		Name       string `json:"name"`
		Context    string `json:"context"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
	} `json:"statusCheckRollup"`
}
type PRListOptions struct {
	State, Search string
	Limit         int
}
type PRDraft struct {
	Number                                       int
	Repository, Head, HeadOID, Base, Title, Body string
	Draft                                        bool
	Labels, Reviewers, RemoveLabels              []string
	Withheld                                     []string // files not sent to the AI: possible secret (VORQ-018)
}

// BranchCommit is one commit of the branch being proposed as a PR.
type BranchCommit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
	Body    string `json:"body,omitempty"`
}

// BranchFileDiff is the diff of one file between the merge base and HEAD.
type BranchFileDiff struct {
	Path    string `json:"path"`
	Diff    string `json:"diff"`
	Omitted bool   `json:"omitted,omitempty"` // diff above the per-file limit, replaced by a notice
}

// BranchChanges is what a PR proposes: commits and diff since the merge base
// with the (locally known) base branch. It never fetches.
type BranchChanges struct {
	Base      string           `json:"base"`
	BaseRef   string           `json:"base_ref"`
	MergeBase string           `json:"merge_base"`
	Commits   []BranchCommit   `json:"commits"`
	Files     []BranchFileDiff `json:"files"`
}

// SyncTarget is the reviewed "bring the base branch" update (VORQ-019).
type SyncTarget struct {
	WorkspacePath, Branch, Remote, Base, HeadHash, SourceID string
	Behind                                                  int
	OnBase, MergeInProgress                                 bool
	Conflicts                                               []string
}
type SyncResult struct {
	Target            SyncTarget
	Incoming          int
	UpToDate, Stopped bool
	Conflicts         []string
	Output            string
}

// PRMergeInfo is GitHub's current view of a PR and of the repository's
// allowed merge methods (VORQ-020).
type PRMergeInfo struct {
	PR               PullRequest
	Mergeable        string // MERGEABLE, CONFLICTING, UNKNOWN
	MergeStateStatus string // CLEAN, BLOCKED, BEHIND, DIRTY, UNSTABLE, …
	Methods          []string
}

// PRMergePlan is the reviewed merge: pinned to HeadOID; Blockers stop it.
type PRMergePlan struct {
	Number                                 int
	Repository, Title, Head, Base, HeadOID string
	Methods, Blockers                      []string
	ChecksPassed, ChecksTotal              int
}
type PRMergeResult struct {
	Plan   PRMergePlan
	Method string
}

// OperationState is a stopped merge, cherry-pick or rebase (VORQ-020).
type OperationState struct {
	Kind      string // "merge", "cherry-pick", "rebase", "revert" or ""
	Conflicts []string
}

// OperationResult is the outcome of cherry-pick, rebase or continue: either
// done, or stopped for resolution (conflicts, or a hook/empty commit).
type OperationResult struct {
	Kind, Output string
	Stopped      bool
	Conflicts    []string
}

// CherryPickPlan pins the commit and the branch it is applied to.
type CherryPickPlan struct {
	WorkspacePath, Branch, HeadHash, Commit, Subject string
}

// RebasePlan replays only unpublished commits onto the remote default branch.
type RebasePlan struct {
	WorkspacePath, Branch, HeadHash, Remote, Base, SourceID string
	Commits, Behind                                         int
}
