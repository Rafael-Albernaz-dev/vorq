package domain

// AIRequest contains the only repository content handed to a provider.
type AIRequest struct {
	Provider string
	Model    string
	Diff     string
}

type AIModel struct {
	ID   string
	Name string
}

// AIProposal binds a suggestion to the exact index reviewed by the core.
type AIProposal struct {
	Proposal CommitProposal
	Diff     string
}
