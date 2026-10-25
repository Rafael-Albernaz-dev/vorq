package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type pullCLIStub struct {
	app.PullService
	calls int
}

func (s *pullCLIStub) GetPullTarget(context.Context, string) (*domain.PullTarget, error) {
	return &domain.PullTarget{Branch: "main", Remote: "origin", RemoteBranch: "main"}, nil
}
func (s *pullCLIStub) PullChanges(context.Context, string, domain.PullTarget) (*domain.PullResult, error) {
	s.calls++
	return &domain.PullResult{Output: "Fast-forward"}, nil
}
func TestPullCLIConfirmation(t *testing.T) {
	for _, input := range []string{"", "n\n", "y\n"} {
		s := &pullCLIStub{}
		c := newPullCmd(s)
		var out bytes.Buffer
		c.SetOut(&out)
		c.SetIn(strings.NewReader(input))
		c.SetArgs(nil)
		err := c.Execute()
		if input == "" && err == nil {
			t.Fatal("EOF authorized fetch")
		}
		want := 0
		if input == "y\n" {
			want = 1
		}
		if s.calls != want {
			t.Fatal("unconfirmed pull")
		}
		if !strings.Contains(out.String(), "origin/main") {
			t.Fatal("source hidden")
		}
	}
}
