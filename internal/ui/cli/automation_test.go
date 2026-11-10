package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type cliAutomationStub struct {
	app.AutomationService
	app.StageAdviceService
	executions int
	plan       *domain.CommitPlan
}

func (s *cliAutomationStub) LoadCommitPlan(context.Context, string) (*domain.CommitPlan, error) {
	return s.plan, nil
}
func (s *cliAutomationStub) ExecuteCommitPlan(_ context.Context, _ string, p *domain.CommitPlan) (*domain.CommitPlan, error) {
	s.executions++
	return p, nil
}

func TestAutomationCLIRequiresExplicitApproval(t *testing.T) {
	for _, test := range []struct {
		name, input string
		args        []string
		executions  int
		wantErr     bool
	}{
		{name: "cancel", input: "n\n", args: []string{"execute"}},
		{name: "EOF", args: []string{"execute"}, wantErr: true},
		{name: "approve", input: "y\n", args: []string{"execute"}, executions: 1},
		{name: "explicit flag", args: []string{"execute", "--yes"}, executions: 1},
		{name: "show", args: []string{"show", "--diff"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &cliAutomationStub{plan: &domain.CommitPlan{ID: "fixture", Commits: []domain.PlannedCommit{{Subject: "feat: reviewed", Files: []string{"F1"}}}, Snapshot: domain.ChangeSnapshot{Files: []domain.SnapshotFile{{ID: "F1", Path: "a.txt", Diff: "PINNED_CONTENT"}}}}}
			cmd := newAICmd(s)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetIn(strings.NewReader(test.input))
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if (err != nil) != test.wantErr || s.executions != test.executions || !strings.Contains(out.String(), "PINNED_CONTENT") {
				t.Fatalf("approval: %v executions=%d output=%s", err, s.executions, out.String())
			}
		})
	}
}
