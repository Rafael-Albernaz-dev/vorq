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
