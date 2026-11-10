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

type prAIStub struct {
	app.StudioService
	calls, saves int
	base         string
}

func (s *prAIStub) DraftPullRequestAI(_ context.Context, _, base, _, _ string) (*domain.PRDraft, error) {
	s.calls++
	s.base = base
	return &domain.PRDraft{Title: "feat: add greeting", Body: "## Summary\nAdds it.", HeadOID: "abc"}, nil
}
func (s *prAIStub) SavePullRequest(context.Context, string, domain.PRDraft) (string, error) {
	s.saves++
	return "", nil
}

func TestPRFormAIDraftFillsOnlyTitleAndBody(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	stub := &prAIStub{StudioService: m.core}
	m.core = stub
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	m.prDraft = domain.PRDraft{Repository: "owner/repo", Head: "feature", Base: "main", Title: "feat: old", Body: "old", Draft: true}
	m.setupPRForm()
	m.fields[2].Value = "develop"          // edited base must be used
	m.fields[3].Value = "bug, enhancement" // labels stay untouched
	if !strings.Contains(ansi.Strip(m.workflowFormView()), prAIButton) {
		t.Fatal("PR form lacks the [IA] button")
	}
	press(m, "ctrl+g")
	if m.modal != "suggest" || m.aiTask != "pr" {
		t.Fatalf("Ctrl+G must open the provider menu, modal=%q task=%q", m.modal, m.aiTask)
	}
	press(m, "esc")
	if m.modal != "workflow" || m.fields[3].Value != "bug, enhancement" {
		t.Fatal("Esc must return to the PR form with its fields intact")
	}
	m.aiProvider, m.aiModels, m.aiModel = "claude", []domain.AIModel{{ID: "opus"}}, "opus"
	press(m, "ctrl+g")
	if m.modal != "ai-send" || !strings.Contains(ansi.Strip(m.modalView()), "Nada é enviado ao GitHub") {
		t.Fatalf("disclosure missing, modal=%q", m.modal)
	}
	driveAll(t, m, press(m, "enter"))
	if stub.calls != 1 || stub.base != "develop" {
		t.Fatalf("calls=%d base=%q", stub.calls, stub.base)
	}
	if m.modal != "workflow" || m.fields[0].Value != "feat: add greeting" || m.fields[1].Value != "## Summary\nAdds it." {
		t.Fatalf("fields not filled: modal=%q %q", m.modal, m.fields[0].Value)
	}
	if m.fields[2].Value != "develop" || m.fields[3].Value != "bug, enhancement" || stub.saves != 0 || m.formReview {
		t.Fatal("AI must not touch other fields nor send the PR")
	}
	assertFrame(t, m)
}
