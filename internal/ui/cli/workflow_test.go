package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type workflowCLIStub struct {
	app.WorkflowService
	app.PullRequestAIService
	aiCalls int
	saves   int
	draft   domain.PRDraft
}

func (s *workflowCLIStub) PreparePullRequest(context.Context, string, string) (*domain.PRDraft, error) {
	return &domain.PRDraft{Repository: "owner/repo", Head: "feature", HeadOID: strings.Repeat("a", 40), Base: "main", Title: "feat: title", Body: "first\nsecond", Draft: true}, nil
}
func (s *workflowCLIStub) SavePullRequest(_ context.Context, _ string, d domain.PRDraft) (string, error) {
	s.saves++
	s.draft = d
	return "https://github.com/owner/repo/pull/1", nil
}
func TestPRCLIRequiresExplicitConfirmationAndPreservesBody(t *testing.T) {
	for _, input := range []string{"", "n\n", "y\n"} {
		stub := &workflowCLIStub{}
		c := newPRSaveCmd(stub, false)
		var out bytes.Buffer
		c.SetIn(strings.NewReader(input))
		c.SetOut(&out)
		c.SetArgs(nil)
		e := c.Execute()
		if input == "" && e == nil {
			t.Fatal("EOF authorized PR")
		}
		want := 0
		if input == "y\n" {
			want = 1
		}
		if stub.saves != want {
			t.Fatalf("input=%q saves=%d", input, stub.saves)
		}
		if want == 1 && stub.draft.Body != "first\nsecond" {
			t.Fatal("body changed")
		}
		if !strings.Contains(out.String(), "owner/repo") {
			t.Fatal("destination not shown")
		}
	}
}
func TestPRCLIYesAndDraftFlag(t *testing.T) {
	stub := &workflowCLIStub{}
	c := newPRSaveCmd(stub, false)
	c.SetOut(&bytes.Buffer{})
	c.SetArgs([]string{"--yes", "--draft=false", "--title", "custom"})
	if e := c.Execute(); e != nil {
		t.Fatal(e)
	}
	if stub.saves != 1 || stub.draft.Draft || stub.draft.Title != "custom" {
		t.Fatal(stub.draft)
	}
}

func (s *workflowCLIStub) DraftPullRequestAI(_ context.Context, _, base, provider, _ string) (*domain.PRDraft, error) {
	s.aiCalls++
	return &domain.PRDraft{Repository: "owner/repo", Head: "feature", Base: base, Title: "feat: AI title (" + provider + ")", Body: "## Summary\nAI body"}, nil
}

// VORQ-016: --ai fills title/body, still asks for confirmation, and explicit
// flags keep precedence over the AI text.
func TestPRCLIAIDraftKeepsConfirmationAndOverrides(t *testing.T) {
	stub := &workflowCLIStub{}
	c := newPRSaveCmd(stub, false)
	var out bytes.Buffer
	c.SetIn(strings.NewReader("n\n"))
	c.SetOut(&out)
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--ai", "--provider", "claude"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if stub.aiCalls != 1 || stub.saves != 0 || !strings.Contains(out.String(), "feat: AI title (claude)") || !strings.Contains(out.String(), "AI body") {
		t.Fatalf("ai=%d saves=%d out=%s", stub.aiCalls, stub.saves, out.String())
	}
	stub = &workflowCLIStub{}
	c = newPRSaveCmd(stub, false)
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--ai", "--yes", "--title", "manual title"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if stub.draft.Title != "manual title" || stub.draft.Body != "## Summary\nAI body" {
		t.Fatalf("override failed: %+v", stub.draft)
	}
}
