package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

type mergeStub struct {
	app.StudioService
	plan    domain.PRMergePlan
	merged  []string
	deleted []bool
	del     domain.BranchDeletePlan
}

func (s *mergeStub) PreparePRMerge(context.Context, string, int) (*domain.PRMergePlan, error) {
	p := s.plan
	return &p, nil
}
func (s *mergeStub) MergePullRequest(_ context.Context, _ string, p domain.PRMergePlan, method string) (*domain.PRMergeResult, error) {
	s.merged = append(s.merged, method)
	return &domain.PRMergeResult{Plan: p, Method: method}, nil
}
func (s *mergeStub) ListPullRequests(context.Context, string, domain.PRListOptions) ([]domain.PullRequest, error) {
	return []domain.PullRequest{{Number: 7, State: "OPEN", Title: "feat: x"}}, nil
}
func (s *mergeStub) PrepareBranchDelete(context.Context, string, string) (*domain.BranchDeletePlan, error) {
	p := s.del
	return &p, nil
}
func (s *mergeStub) DeleteBranch(_ context.Context, _ string, p domain.BranchDeletePlan, confirmed bool) (*domain.BranchDeleteResult, error) {
	s.deleted = append(s.deleted, confirmed)
	return &domain.BranchDeleteResult{Plan: p}, nil
}

func TestPRMergeReviewMethodAndCleanup(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	plan := domain.PRMergePlan{Number: 7, Title: "feat: x", Head: "feat/x", Base: "main", HeadOID: "abc123", Repository: "owner/repo", Methods: []string{"merge", "squash"}, Blockers: []string{"1 check(s) falhando"}}
	stub := &mergeStub{StudioService: m.core, plan: plan, del: domain.BranchDeletePlan{Branch: "feat/x", Hash: "abc123", RemoteHash: "abc123", Remote: "origin", Base: "main"}}
	m.core = stub
	m.screen = "prs"
	m.prs = []domain.PullRequest{{Number: 7, State: "OPEN", Title: "feat: x"}}
	if !strings.Contains(ansi.Strip(m.workflowView()), "M mergear") {
		t.Fatal("shortcut missing")
	}
	driveAll(t, m, press(m, "M"))
	if m.modal != modalPRMerge || !strings.Contains(ansi.Strip(m.modalView()), "check(s) falhando") {
		t.Fatalf("blocked review: modal %q", m.modal)
	}
	driveAll(t, m, press(m, "enter"))
	if len(stub.merged) != 0 {
		t.Fatal("blocked PR merged")
	}
	press(m, "esc")
	stub.plan.Blockers = nil
	driveAll(t, m, press(m, "M"))
	press(m, "2")
	assertFrame(t, m)
	driveAll(t, m, press(m, "enter"))
	if strings.Join(stub.merged, ",") != "squash" || m.modal != modalResult || m.resultKind != "pr-merge" || m.prMerged == nil {
		t.Fatalf("merged %v modal %q notice %q", stub.merged, m.modal, m.notice)
	}
	// d: the branch tip is exactly the merged head, so no CONFIRMAR is needed
	// even though a squash merge leaves it "unmerged" locally.
	driveAll(t, m, press(m, "d"))
	if m.modal != modalBranchDelete || m.branchDeleteNeedsTyping() || !strings.Contains(ansi.Strip(m.modalView()), "mergeado no PR #7") {
		t.Fatalf("cleanup delete: modal %q", m.modal)
	}
	driveAll(t, m, press(m, "enter"))
	if len(stub.deleted) != 1 || !stub.deleted[0] {
		t.Fatalf("deleted %v", stub.deleted)
	}
}
