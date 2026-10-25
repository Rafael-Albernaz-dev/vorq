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
