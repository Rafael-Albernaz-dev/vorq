package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type prMergeStub struct {
	plan   domain.PRMergePlan
	merged []string
}

func (s *prMergeStub) PreparePRMerge(context.Context, string, int) (*domain.PRMergePlan, error) {
	p := s.plan
	return &p, nil
}
func (s *prMergeStub) MergePullRequest(_ context.Context, _ string, p domain.PRMergePlan, method string) (*domain.PRMergeResult, error) {
	s.merged = append(s.merged, method)
	return &domain.PRMergeResult{Plan: p, Method: method}, nil
}
func (s *prMergeStub) ListLocalBranches(context.Context, string) ([]domain.LocalBranch, error) {
	return nil, nil
}
func (s *prMergeStub) PrepareBranchSwitch(context.Context, string, string) (*domain.BranchSwitchPlan, error) {
	return &domain.BranchSwitchPlan{}, nil
}
func (s *prMergeStub) SwitchBranch(context.Context, string, domain.BranchSwitchPlan) error {
	return nil
}
func (s *prMergeStub) GetPullTarget(context.Context, string) (*domain.PullTarget, error) {
	return &domain.PullTarget{}, nil
}
func (s *prMergeStub) PullChanges(context.Context, string, domain.PullTarget) (*domain.PullResult, error) {
	return &domain.PullResult{}, nil
}
func (s *prMergeStub) PrepareBranchDelete(context.Context, string, string) (*domain.BranchDeletePlan, error) {
	return &domain.BranchDeletePlan{Branch: "feat/x", Hash: "abc", Merged: true}, nil
}
func (s *prMergeStub) DeleteBranch(_ context.Context, _ string, p domain.BranchDeletePlan, _ bool) (*domain.BranchDeleteResult, error) {
	return &domain.BranchDeleteResult{Plan: p}, nil
}

func runPRMerge(t *testing.T, stub *prMergeStub, input string, args ...string) (string, error) {
	t.Helper()
	c := newPRMergeCmd(stub)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetIn(strings.NewReader(input))
	c.SetArgs(args)
	err := c.Execute()
	return out.String(), err
}

func TestPRMergeCLI(t *testing.T) {
	base := domain.PRMergePlan{Number: 7, Title: "feat: x", Head: "feat/x", Base: "main", HeadOID: "abc123", Repository: "owner/repo", Methods: []string{"squash", "merge"}}
	blocked := base
	blocked.Blockers = []string{"1 check(s) falhando"}
	stub := &prMergeStub{plan: blocked}
	out, err := runPRMerge(t, stub, "y\n", "7")
	if err == nil || len(stub.merged) != 0 || !strings.Contains(out, "check(s) falhando") {
		t.Fatalf("blocked: %v %v %q", err, stub.merged, out)
	}
	stub = &prMergeStub{plan: base}
	if out, err = runPRMerge(t, stub, "n\n", "7"); err != nil || len(stub.merged) != 0 {
		t.Fatalf("declined: %v %v %q", err, stub.merged, out)
	}
	if _, err = runPRMerge(t, stub, "", "7", "--yes", "--method", "rebase"); err == nil || len(stub.merged) != 0 {
		t.Fatalf("disallowed method: %v", err)
	}
	out, err = runPRMerge(t, stub, "y\nn\nn\n", "7")
	if err != nil || strings.Join(stub.merged, ",") != "squash" || !strings.Contains(out, "PR #7 mergeado em main (squash)") {
		t.Fatalf("merge: %v %v %q", err, stub.merged, out)
	}
}
