package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type workflowStub struct {
	app.StudioService
	lists, saves, views int
	saved               domain.PRDraft
}

func (s *workflowStub) ListPullRequests(context.Context, string, domain.PRListOptions) ([]domain.PullRequest, error) {
	s.lists++
	return []domain.PullRequest{{Number: 12, Title: "test PR", Head: "feature", Base: "main", State: "OPEN"}}, nil
}
func (s *workflowStub) ViewPullRequest(context.Context, string, int) (*domain.PullRequest, error) {
	s.views++
	return &domain.PullRequest{Number: 12, Title: "test PR", Body: "body\nsecond", Head: "feature", Base: "main", State: "OPEN"}, nil
}
func (s *workflowStub) PreparePullRequest(context.Context, string, string) (*domain.PRDraft, error) {
	return &domain.PRDraft{Repository: "owner/repo", Head: "feature", Base: "main", Title: "feat: new", Body: "body\nsecond", Draft: true}, nil
}
func (s *workflowStub) SavePullRequest(_ context.Context, _ string, d domain.PRDraft) (string, error) {
	s.saves++
	s.saved = d
	return "https://github.com/owner/repo/pull/12", nil
}
func TestWorkflowBranchRequiresReviewAndPreservesCommitDraft(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	m.commitSummary = "existing draft"
	drive(t, m, press(m, "b"))
	press(m, "feat/q-branch")
	press(m, "enter")
	branch, _ := r.CurrentBranch(context.Background(), dir)
	if branch != "main" {
		t.Fatal("branch created before review")
	}
	press(m, "ctrl+s")
	if !m.formReview {
		t.Fatal("review missing")
	}
	drive(t, m, press(m, "enter"))
	branch, _ = r.CurrentBranch(context.Background(), dir)
	if branch != "feat/q-branch" || m.commitSummary != "existing draft" {
		t.Fatalf("branch=%s draft=%s", branch, m.commitSummary)
	}
}
func TestWorkflowHistoryAndLateResponses(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	drive(t, m, press(m, "H"))
	if m.screen != "history" || m.history == nil || m.history.Count < 1 || !strings.Contains(m.detailText, "chore: initial") {
		t.Fatalf("history not loaded: %+v", m.history)
	}
	id := m.workflowID
	press(m, "esc")
	m.Update(workflowMsg{id: id, dir: m.dir, kind: "history", page: &domain.HistoryPage{Count: 999}})
	if m.screen != "" || m.history.Count == 999 {
		t.Fatal("late history response restored screen")
	}
	m.detailText = "preserve"
	m.Update(workflowDetailMsg{id: m.detailID - 1, text: "old"})
	if m.detailText != "preserve" {
		t.Fatal("late detail accepted")
	}
}
func TestWorkflowPRReadsExplicitAndSaveConfirmedOnce(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	stub := &workflowStub{StudioService: m.core}
	m.core = stub
	m.studio.Repository = &domain.RepositoryLink{URL: "https://github.com/owner/repo", Slug: "owner/repo"}
	drive(t, m, press(m, "P"))
	if stub.lists != 1 || stub.views != 0 {
		t.Fatal("list triggered implicit detail calls")
	}
	drive(t, m, press(m, "enter"))
	if stub.views != 1 || !strings.Contains(m.detailText, "second") {
		t.Fatal("PR detail unavailable")
	}
	drive(t, m, press(m, "c"))
	if m.modal != "workflow" || m.fields[0].Value != "feat: new" {
		t.Fatal("draft not prefilled")
	}
	press(m, "q")
	if m.fields[0].Value != "feat: newq" {
		t.Fatal("q interpreted as quit in form")
	}
	press(m, "ctrl+s")
	if stub.saves != 0 {
		t.Fatal("review published PR")
	}
	cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("save missing")
	}
	if second := press(m, "enter"); second != nil {
		t.Fatal("double submission possible")
	}
	drive(t, m, cmd)
	if stub.saves != 1 || stub.saved.Title != "feat: newq" || !stub.saved.Draft {
		t.Fatalf("save %+v", stub.saved)
	}
	if m.mutating || m.busy != "" {
		t.Fatal("operation did not settle")
	}
}
func TestWorkflowLayoutsAndCredentialFreeCenteredHeader(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.studio.Repository = &domain.RepositoryLink{URL: "https://github.com/owner/repo", Slug: "owner/repo"}
	for _, size := range [][2]int{{50, 14}, {80, 24}, {120, 30}, {160, 45}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		header := ansi.Strip(m.repositoryHeader())
		if !strings.Contains(header, "vorq / github.com/owner/repo") {
			t.Fatal(header)
		}
		m.prDraft = domain.PRDraft{Repository: "owner/repo", Head: "feature", Base: "main", Title: "feat: title", Body: strings.Repeat("long body\n", 50)}
		m.setupPRForm()
		for _, review := range []bool{false, true} {
			m.formReview = review
			rows := strings.Split(m.workflowFormView(), "\n")
			if len(rows) > m.height {
				t.Fatalf("%v review=%t rows=%d exceeds %d", size, review, len(rows), m.height)
			}
			for _, line := range rows {
				if ansi.StringWidth(line) > m.width {
					t.Fatal("form overflows width")
				}
			}
			if !strings.Contains(ansi.Strip(m.workflowFormView()), "[Cancelar]") {
				t.Fatal("cancel hidden")
			}
		}
		m.modal = ""
		m.screen = "prs"
		m.prs = []domain.PullRequest{{Number: 1, Title: "test"}}
		m.selectPR()
		rows := strings.Split(m.workflowView(), "\n")
		if len(rows) != m.height {
			t.Fatalf("screen height %d != %d", len(rows), m.height)
		}
		for _, line := range rows {
			if ansi.StringWidth(line) > m.width {
				t.Fatal("screen overflows width")
			}
		}
		m.screen = ""
	}
}

func TestWorkflowRealSpaceAndMultilineEditing(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.prDraft = domain.PRDraft{Title: "title", Body: "first\nsecond", Base: "main"}
	m.setupPRForm()
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.fields[0].Value != "title " {
		t.Fatal("terminal space lost")
	}
	press(m, "tab")
	press(m, "home")
	press(m, "up")
	press(m, "q")
	if m.fields[1].Value != "qfirst\nsecond" {
		t.Fatalf("multiline cursor incorrect: %q", m.fields[1].Value)
	}
	press(m, "tab")
	press(m, "ctrl+u")
	press(m, "develop")
	if !strings.Contains(ansi.Strip(m.workflowFormView()), "→ develop") {
		t.Fatal("review destination header stale")
	}
}
