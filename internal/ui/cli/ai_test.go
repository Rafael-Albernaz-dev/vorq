package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type cliAIService struct {
	app.Service
	provider string
	fail     bool
}

func (s *cliAIService) SuggestCommitAI(_ context.Context, _ string, p string, _ ...string) (*domain.AIProposal, error) {
	s.provider = p
	if s.fail {
		return nil, fmt.Errorf("quota exceeded")
	}
	return &domain.AIProposal{Proposal: domain.CommitProposal{Subject: "fix: preserve staged content", Body: "Describe the staged change."}}, nil
}
func (s *cliAIService) ProposeCommit(_ context.Context, _ string, typ string) (*domain.CommitProposal, error) {
	s.provider = "rules"
	return &domain.CommitProposal{Subject: typ + ": update staged files"}, nil
}

func TestSuggestCLIProvidersAndRecovery(t *testing.T) {
	for _, provider := range []string{"codex", "gemini", "rules"} {
		t.Run(provider, func(t *testing.T) {
			s := &cliAIService{}
			cmd := newSuggestCmd(s)
			var out, diagnostics bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&diagnostics)
			cmd.SetArgs([]string{"--provider", provider})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if s.provider != provider || out.Len() == 0 {
				t.Fatal("wrong provider or empty result")
			}
			if provider != "rules" && !strings.Contains(diagnostics.String(), "staged diff") {
				t.Fatal("missing disclosure")
			}
		})
	}
	s := &cliAIService{fail: true}
	cmd := newSuggestCmd(s)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--provider", "gemini"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--provider rules") || s.provider != "gemini" {
		t.Fatal("missing explicit recovery or silent provider change")
	}
	s = &cliAIService{}
	cmd = newSuggestCmd(s)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--provider", "invalid"})
	if err := cmd.Execute(); err == nil || s.provider != "" {
		t.Fatal("invalid provider invoked")
	}
}
