package tui

import (
	"context"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type workflowState struct {
	screen, returnScreen         string
	workflowID, detailID         uint64
	workflowCancel, detailCancel context.CancelFunc
	history                      *domain.HistoryPage
	historyOptions               domain.HistoryOptions
	historyIndex, historyOffset  int
	prs                          []domain.PullRequest
	prOptions                    domain.PRListOptions
	prIndex, prOffset            int
	associations                 map[string][]domain.PullRequest
	detailText                   string
	detailOffset                 int
	formKind                     string
	fields                       []workflowField
	fieldIndex                   int
	formReview                   bool
	prDraft                      domain.PRDraft
}
type workflowField struct {
	Label, Value      string
	Cursor            int
	Multiline, Toggle bool
}
type workflowMsg struct {
	id        uint64
	kind, dir string
	page      *domain.HistoryPage
	prs       []domain.PullRequest
	draft     *domain.PRDraft
	pr        *domain.PullRequest
	err       error
}
type workflowDetailMsg struct {
	id   uint64
	text string
	err  error
}
