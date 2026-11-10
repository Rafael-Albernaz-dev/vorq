package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

type readyStub struct {
	app.StudioService
	ready []int
}

func (s *readyStub) MarkPullRequestReady(_ context.Context, _ string, n int) error {
	s.ready = append(s.ready, n)
	return nil
}
func (s *readyStub) ListPullRequests(context.Context, string, domain.PRListOptions) ([]domain.PullRequest, error) {
	return []domain.PullRequest{{Number: 7, State: "OPEN", Title: "feat: x"}}, nil
}

func TestPRScreenMarksDraftReadyAfterConfirmation(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	stub := &readyStub{StudioService: m.core}
	m.core = stub
	m.screen = "prs"
	m.prs = []domain.PullRequest{{Number: 7, State: "OPEN", Draft: true, Title: "feat: x"}, {Number: 8, State: "OPEN", Title: "fix: y"}}
	if !strings.Contains(ansi.Strip(m.workflowView()), "OPEN · DRAFT") || !strings.Contains(ansi.Strip(m.workflowView()), "y pronto") {
		t.Fatal("draft marker or shortcut missing")
	}
	press(m, "y")
	if m.modal != modalPRReady || !strings.Contains(ansi.Strip(m.modalView()), "#7") {
		t.Fatalf("confirmation missing: modal=%q", m.modal)
	}
	if len(stub.ready) != 0 {
		t.Fatal("marked before confirmation")
	}
	driveAll(t, m, press(m, "enter"))
	if len(stub.ready) != 1 || stub.ready[0] != 7 {
		t.Fatalf("ready calls %v", stub.ready)
	}
	m.prIndex = 1 // not a draft
	if press(m, "y"); m.modal == modalPRReady {
		t.Fatal("non-draft PR offered as ready")
	}
}
