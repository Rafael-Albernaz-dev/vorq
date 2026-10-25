package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type branchSwitchStub struct {
	app.BranchSwitchService
	switches int
}

func (s *branchSwitchStub) PrepareBranchSwitch(context.Context, string, string) (*domain.BranchSwitchPlan, error) {
	return &domain.BranchSwitchPlan{From: "main", Target: "feature"}, nil
}
func (s *branchSwitchStub) SwitchBranch(context.Context, string, domain.BranchSwitchPlan) error {
	s.switches++
	return nil
}
func TestSwitchBranchCLIConfirmation(t *testing.T) {
	for _, input := range []string{"", "n\n", "y\n"} {
		s := &branchSwitchStub{}
		cmd := newSwitchCmd(s)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetIn(strings.NewReader(input))
		cmd.SetArgs([]string{"feature"})
		err := cmd.Execute()
		if input == "" && err == nil {
			t.Fatal("EOF switched")
		}
		want := 0
		if input == "y\n" {
			want = 1
		}
		if s.switches != want || !strings.Contains(out.String(), "main -> feature") {
			t.Fatal("confirmation or preview incorrect")
		}
	}
}
