package domain

type SnapshotFile struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	OldPath     string `json:"old_path,omitempty"`
	Mode        string `json:"mode"`
	Content     []byte `json:"content,omitempty"`
	Diff        string `json:"diff"`
	Fingerprint string `json:"fingerprint"`
	Excluded    string `json:"excluded,omitempty"`
}
type ChangeSnapshot struct {
	ID        string         `json:"id"`
	Workspace string         `json:"workspace"`
	Head      string         `json:"head"`
	Index     string         `json:"index"`
	Files     []SnapshotFile `json:"files"`
}
type ReviewFinding struct {
	FileID     string `json:"file_id"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Evidence   string `json:"evidence"`
	Suggestion string `json:"suggestion,omitempty"`
}
type DiffReview struct {
	Summary  string          `json:"summary"`
	Findings []ReviewFinding `json:"findings"`
	Dropped  int             `json:"dropped,omitempty"` // invalid findings removed by validation
	Snapshot ChangeSnapshot  `json:"snapshot"`
}
type PlannedCommit struct {
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	Reason  string   `json:"reason"`
	Files   []string `json:"files"`
}
type PlanStep struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}
type CommitPlan struct {
	ID            string          `json:"id"`
	Snapshot      ChangeSnapshot  `json:"snapshot"`
	Commits       []PlannedCommit `json:"commits"`
	Completed     []PlanStep      `json:"completed"`
	ExpectedHead  string          `json:"expected_head"`
	ExpectedIndex string          `json:"expected_index"`
	Error         string          `json:"error,omitempty"`
}
type AnalysisRequest struct{ Provider, Model, Task, Payload string }

type PlanEdit struct {
	Kind    string
	Group   int
	Target  int
	FileID  string
	Subject string
	Body    string
}
