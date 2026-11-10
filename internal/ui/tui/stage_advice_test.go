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

type stageAdviceStub struct {
	app.StudioService
	applied int
}

func (s *stageAdviceStub) StageAdviceAI(context.Context, string, string, string) (*domain.StageAdvice, error) {
	return &domain.StageAdvice{
		Summary: "One feature and one unrelated debug file.",
		Stage:   []domain.StageAdviceItem{{FileID: "F0001", Reason: "Implements the feature"}},
		Hold:    []domain.StageAdviceItem{{FileID: "F0002", Reason: "Debug output", Risk: "Leaks logs"}},
		Subject: "feat: add greeting", Body: "Print a greeting.",
		Context:  []string{"README.md"},
		Changes:  []domain.StageChange{{Path: "file.go", Action: "stage"}, {Path: "debug.go", Action: "unstage", Partial: true}},
		Snapshot: domain.ChangeSnapshot{Files: []domain.SnapshotFile{{ID: "F0001", Path: "file.go"}, {ID: "F0002", Path: "debug.go"}}},
	}, nil
}
func (s *stageAdviceStub) ApplyStageAdvice(context.Context, string, *domain.StageAdvice) error {
	s.applied++
	return nil
}

func TestStageAdviceFlowAppliesAfterConfirmationAndFillsDraft(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	stub := &stageAdviceStub{StudioService: m.core}
	m.core = stub
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m.aiProvider, m.aiModels, m.aiModel = "claude", []domain.AIModel{{ID: "haiku"}}, "haiku"
	m.commitSummary = "feat: my draft"
	if cmd := m.openAIAction("stage"); cmd != nil || m.modal != "ai-send" {
		t.Fatalf("stage should open the disclosure, modal=%q", m.modal)
	}
	if !strings.Contains(ansi.Strip(m.modalView()), "documentos de contexto versionados") {
		t.Fatal("disclosure must mention context documents")
	}
	driveAll(t, m, press(m, "enter"))
	view := ansi.Strip(m.modalView())
	for _, want := range []string{"PREPARAR NO PRÓXIMO COMMIT", "DEIXAR DE FORA", "MENSAGEM SUGERIDA", "file.go", "debug.go", "Risco: Leaks logs", "Contexto lido: README.md", applyButton} {
		if !strings.Contains(view, want) {
			t.Errorf("advice view missing %q", want)
		}
	}
	press(m, "a")
	confirm := ansi.Strip(m.modalView())
	if m.modal != "stage-confirm" || !strings.Contains(confirm, "entra no stage") || !strings.Contains(confirm, "sai do stage") || !strings.Contains(confirm, "parcialmente") || !strings.Contains(confirm, "rascunho atual será substituído") {
		t.Fatalf("confirmation must list every change:\n%s", confirm)
	}
	if stub.applied != 0 {
		t.Fatal("applied before confirmation")
	}
	driveAll(t, m, press(m, "enter"))
	if stub.applied != 1 || m.commitSummary != "feat: add greeting" || m.commitDescription != "Print a greeting." {
		t.Fatalf("apply=%d draft=%q/%q", stub.applied, m.commitSummary, m.commitDescription)
	}
	assertFrame(t, m)
}

func TestStageAdviceIsInTheAITab(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.rightTab = "ai"
	found := false
	for _, a := range m.panelActions() {
		found = found || a.key == "ai-stage"
	}
	if !found {
		t.Fatal("IA tab lacks Preparar próximo commit")
	}
}
