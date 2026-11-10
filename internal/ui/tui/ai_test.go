package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

type aiStudioService struct {
	app.StudioService
	calls   int
	invalid bool
	model   string
}

func (s *aiStudioService) ListAIModels(_ context.Context, provider string) ([]domain.AIModel, error) {
	id := "gpt-test"
	if provider == "gemini" {
		id = "gemini-test"
	}
	return []domain.AIModel{{ID: "", Name: "Padrão"}, {ID: id, Name: "Modelo de teste"}}, nil
}

func TestAISuggestionMouseOwnership(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	s := &aiStudioService{StudioService: core}
	m.core = s
	for _, size := range [][2]int{{140, 40}, {80, 24}, {50, 14}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.openSuggestion()
		x, y := mouseLabelPosition(t, m.View(), "[c]")
		before := s.calls
		drive(t, m, mousePress(m, x+1, y))
		if m.modal != "ai-models" || s.calls != before {
			t.Fatal("provider click submitted")
		}
		x, y = mouseLabelPosition(t, m.View(), "[2]")
		mousePress(m, x+1, y)
		x, y = mouseLabelPosition(t, m.View(), "[Revisar]")
		drive(t, m, mousePress(m, x+1, y))
		if m.aiModel != "gpt-test" || m.modal != "ai-send" {
			t.Fatal("model selection failed")
		}
		x, y = mouseLabelPosition(t, m.View(), "[Confirmar]")
		drive(t, m, mousePress(m, x+1, y))
		if m.modal != "ai-review" || s.calls != before+1 || s.model != "gpt-test" {
			t.Fatal("send click failed")
		}
		press(m, "o")
		x, y = mouseLabelPosition(t, m.View(), "[r]")
		drive(t, m, mousePress(m, x+1, y))
		if m.modal != "ai-send" || s.calls != before+1 {
			t.Fatal("retry click submitted without confirmation")
		}
		press(m, "esc")
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.modal = "ai-review"
	m.aiProposal = &domain.AIProposal{Proposal: domain.CommitProposal{Subject: "fix: literals [Confirmar] [Opções] [c] [g]"}}
	for _, token := range []string{"[Confirmar]", "[Opções]", "[c]", "[g]"} {
		x, y := mouseLabelPosition(t, m.View(), token)
		if cmd := mousePress(m, x+1, y); cmd != nil || m.modal != "ai-review" {
			t.Fatalf("AI text became an action: %s", token)
		}
	}
}

func (s *aiStudioService) SuggestCommitAI(_ context.Context, _ string, _ string, models ...string) (*domain.AIProposal, error) {
	s.calls++
	if len(models) > 0 {
		s.model = models[0]
	}
	return &domain.AIProposal{Proposal: domain.CommitProposal{Subject: "fix: preserve staged changes", Body: "Explain only the staged changes."}, Diff: "reviewed"}, nil
}
func (s *aiStudioService) ValidateAISuggestion(context.Context, string, string) error {
	if s.invalid {
		return fmt.Errorf("stage changed")
	}
	return nil
}

func TestAISuggestionReviewApplyCancelAndStale(t *testing.T) {
	m, core, _, _, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	s := &aiStudioService{StudioService: core}
	m.core = s
	m.commitSummary, m.commitDescription = "human title", "human body"
	m.openSuggestion()
	drive(t, m, press(m, "c"))
	if m.modal != "ai-models" || s.calls != 0 {
		t.Fatal("selection sent data without review")
	}
	press(m, "enter")
	drive(t, m, press(m, "enter"))
	if m.modal != "ai-review" || s.calls != 1 || m.commitSummary != "human title" || m.commitDescription != "human body" {
		t.Fatal("preview changed draft")
	}
	press(m, "esc")
	if m.commitDescription != "human body" {
		t.Fatal("cancel changed draft")
	}
	m.openSuggestion()
	drive(t, m, press(m, "g"))
	press(m, "enter")
	drive(t, m, press(m, "enter"))
	s.invalid = true
	drive(t, m, press(m, "enter"))
	if m.modal != "ai-error" || m.commitSummary != "human title" {
		t.Fatal("stale stage overwrote draft")
	}
	press(m, "r")
	s.invalid = false
	drive(t, m, press(m, "enter"))
	drive(t, m, press(m, "enter"))
	if m.modal != "" || m.commitSummary != "fix: preserve staged changes" || m.commitDescription != "Explain only the staged changes." {
		t.Fatal("reviewed apply failed")
	}
	old := aiProposalMsg{id: m.dialogID, revision: m.draftRevision, proposal: &domain.AIProposal{Proposal: domain.CommitProposal{Subject: "fix: old response"}}}
	m.modal = "ai-send"
	press(m, "esc")
	m.Update(old)
	if m.commitSummary == "fix: old response" {
		t.Fatal("old response applied")
	}
	m.modal = "ai-error"
	press(m, "l")
	if m.modal != "suggest" {
		t.Fatal("local rules recovery unavailable")
	}
	for _, size := range [][2]int{{140, 45}, {80, 24}, {50, 14}} {
		m.width, m.height = size[0], size[1]
		for _, modal := range []modalID{"suggest", "ai-models", "ai-send", "ai-review", "ai-error"} {
			m.modal = modal
			v := m.modalView()
			if v == "" || !strings.Contains(v, "Voltar") {
				t.Fatalf("missing controls %s at %v", modal, size)
			}
		}
	}
}

func TestAIModelSuggestionToReviewedCommit(t *testing.T) {
	m, core, runner, dir, _ := setupStudio(t)
	drive(t, m, press(m, "s"))
	s := &aiStudioService{StudioService: core}
	m.core = s
	m.openSuggestion()
	drive(t, m, press(m, "c"))
	press(m, "2")
	press(m, "enter")
	drive(t, m, press(m, "enter"))
	if m.modal != "ai-review" || s.model != "gpt-test" {
		t.Fatal("model not used")
	}
	drive(t, m, press(m, "v"))
	if m.modal != "commit" || m.commitPhase != "review" || core.commitCalls != 0 {
		t.Fatal("AI bypassed commit review")
	}
	drive(t, m, press(m, "enter"))
	if core.commitCalls != 1 || core.pushCalls != 0 {
		t.Fatal("wrong commit/push behavior")
	}
	out, err := runner.Exec(context.Background(), dir, "log", "-1", "--format=%s")
	if err != nil || strings.TrimSpace(out) != "fix: preserve staged changes" {
		t.Fatalf("wrong committed suggestion: %s %v", out, err)
	}
}

func TestAISuggestionVisibleWithoutStageInCompactView(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 14})
	if m.changeSummary().Staged != 0 {
		t.Fatal("fixture unexpectedly staged")
	}
	x, y := mouseLabelPosition(t, m.View(), "[IA] Sugerir")
	if y != m.height-1 {
		t.Fatal("AI entry not visible in footer")
	}
	mousePress(m, x+1, y)
	if m.modal != "suggest" {
		t.Fatal("AI options hidden without stage")
	}
}
