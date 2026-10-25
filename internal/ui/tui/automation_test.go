package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
	tea "github.com/charmbracelet/bubbletea"
)

type automationAI struct{ calls int }

func (p *automationAI) Suggest(context.Context, domain.AIRequest) (*domain.CommitProposal, error) {
	panic("unexpected suggestion")
}
func (p *automationAI) Analyze(_ context.Context, req domain.AnalysisRequest) (json.RawMessage, error) {
	p.calls++
	if req.Task == "review" {
		return json.RawMessage(`{"summary":"Inspect changes [Confirmar]","findings":[]}`), nil
	}
	var files []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(req.Payload), &files); err != nil {
		return nil, err
	}
	var commits []domain.PlannedCommit
	for _, f := range files {
		commits = append(commits, domain.PlannedCommit{Subject: "feat: implement reviewed file", Files: []string{f.ID}})
	}
	return json.Marshal(map[string]any{"commits": commits})
}

func TestAutomationTUIReviewEditAndExplicitExecution(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _, runner, dir, _ := setupStudio(t)
	a := app.NewApp(git.NewRepository(runner))
	ai := &automationAI{}
	a.SetAIProvider(ai)
	m.core = a
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 42})
	m.aiProvider = "codex"
	m.aiModels = []domain.AIModel{{ID: "gpt-test", Name: "Test"}}
	m.aiModel = "gpt-test"
	head, _ := runner.Exec(context.Background(), dir, "rev-parse", "HEAD")
	clickPanelTab(t, m, 1)
	if ai.calls != 0 {
		t.Fatal("tab performed network")
	}
	drive(t, m, clickPanelAction(t, m, "Revisar diff"))
	if m.modal != "ai-send" || ai.calls != 0 {
		t.Fatal("missing send confirmation")
	}
	drive(t, m, press(m, "enter"))
	if m.modal != "diff-review" || ai.calls != 1 {
		t.Fatal("review not presented")
	}
	press(m, "o")
	drive(t, m, press(m, "enter"))
	if m.modal != "plan" || ai.calls != 2 || m.commitPlan == nil {
		t.Fatal("plan missing")
	}
	current, _ := runner.Exec(context.Background(), dir, "rev-parse", "HEAD")
	if current != head {
		t.Fatal("analysis committed")
	}
	press(m, "e")
	press(m, "ctrl+u")
	press(m, "feat: user approved subject")
	drive(t, m, press(m, "ctrl+s"))
	if m.modal != "plan" || m.commitPlan.Commits[0].Subject != "feat: user approved subject" {
		t.Fatal("message edit lost")
	}
	press(m, "d")
	if !strings.Contains(m.planDiffBody(), "// changed") {
		t.Fatal("pinned diff missing")
	}
	press(m, "esc")
	press(m, "enter")
	if m.modal != "plan-execute" {
		t.Fatal("execution confirmation absent")
	}
	current, _ = runner.Exec(context.Background(), dir, "rev-parse", "HEAD")
	if current != head {
		t.Fatal("commit before final approval")
	}
	press(m, "esc")
	m.modal = "plan"
	press(m, "enter")
	cmd := press(m, "enter")
	if !m.mutating || cmd == nil || press(m, "enter") != nil {
		t.Fatal("duplicate execution")
	}
	drive(t, m, cmd)
	if len(m.commitPlan.Completed) != 1 {
		t.Fatalf("execution failed: %s", m.notice)
	}
	subject, _ := runner.Exec(context.Background(), dir, "log", "-1", "--format=%s")
	if strings.TrimSpace(subject) != "feat: user approved subject" {
		t.Fatal("unreviewed subject")
	}
	for _, size := range [][2]int{{160, 42}, {80, 24}, {50, 14}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, modal := range []string{"plan", "plan-diff", "plan-edit", "plan-execute", "diff-review"} {
			m.modal = modal
			if !strings.Contains(m.modalView(), "Voltar") && !strings.Contains(m.modalView(), "Cancelar") {
				t.Fatalf("no exit: %s %v", modal, size)
			}
			assertFrame(t, m)
		}
	}
}

func TestRightClickCopiesClickedFileWithoutSelectionOrGitMutation(t *testing.T) {
	m, core, runner, dir, _ := setupStudio(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	writeStudio(t, dir, "nome longo ação [x].txt", "reference\n")
	drive(t, m, m.refresh())
	drive(t, m, press(m, "e"))
	selectPath(t, m, "file.go")
	before, _ := runner.Exec(context.Background(), dir, "status", "--porcelain", "-z")
	selected := m.selected
	copyDir := t.TempDir()
	out := filepath.Join(copyDir, "out")
	t.Setenv("WAYLAND_DISPLAY", "test")
	t.Setenv("DISPLAY", "")
	t.Setenv("COPY_OUTPUT", out)
	t.Setenv("PATH", copyDir+":"+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(copyDir, "wl-copy"), []byte("#!/bin/sh\n/bin/cat > \"$COPY_OUTPUT\"\n"), 0700)
	index := -1
	for i, row := range m.rows {
		if row.node != nil && row.node.Path == "nome longo ação [x].txt" {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("fixture not visible")
	}
	m.treeOffset = max(0, index-2)
	msg := tea.MouseMsg{X: 3, Y: m.bodyTop() + 4 + index - m.treeOffset, Button: tea.MouseButtonRight, Action: tea.MouseActionPress}
	_, cmd := m.Update(msg)
	drive(t, m, cmd)
	data, _ := os.ReadFile(out)
	if string(data) != filepath.Join(dir, "nome longo ação [x].txt") || m.selected != selected {
		t.Fatalf("wrong clicked path/selection: %q", data)
	}
	for _, action := range []tea.MouseAction{tea.MouseActionRelease, tea.MouseActionMotion} {
		msg.Action = action
		_, cmd = m.Update(msg)
		if cmd != nil {
			t.Fatal("non-press copied")
		}
	}
	after, _ := runner.Exec(context.Background(), dir, "status", "--porcelain", "-z")
	if before != after || core.stageCalls != 0 || core.commitCalls != 0 {
		t.Fatal("copy mutated Git")
	}
}
