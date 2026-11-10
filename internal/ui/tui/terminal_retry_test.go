package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// pushStub fails the first push only because a prompt was needed, and records
// whether each attempt was allowed to use the terminal.
type pushStub struct {
	app.StudioService
	attempts []bool
}

func (s *pushStub) GetPushTarget(context.Context, string) (*domain.PushTarget, error) {
	return &domain.PushTarget{Branch: "main", Remote: "origin", RemoteBranch: "main", HasUpstream: true}, nil
}
func (s *pushStub) PushChanges(ctx context.Context, _ string, opts domain.PushOptions) (*domain.PushResult, error) {
	s.attempts = append(s.attempts, domain.TerminalAllowed(ctx))
	if len(s.attempts) == 1 {
		return nil, &domain.TerminalRequiredError{Reason: "credencial necessária", Err: errors.New("terminal prompts disabled")}
	}
	return &domain.PushResult{Target: opts.Target}, nil
}

func TestStudioCommandsNeverUseTheTerminal(t *testing.T) {
	m := NewModel(context.Background(), nil, "/workspace")
	if domain.TerminalAllowed(m.ctx) {
		t.Fatal("Studio context must disable terminal prompts")
	}
}

func TestPushNeedingCredentialsOffersTerminalRetry(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	stub := &pushStub{StudioService: m.core}
	m.core = stub
	drive(t, m, press(m, "p")) // review target
	drive(t, m, press(m, "enter"))
	if len(stub.attempts) != 1 || stub.attempts[0] {
		t.Fatalf("first push must run without terminal: %v", stub.attempts)
	}
	if m.modal != "error" || m.terminalRetryKind != "push" {
		t.Fatalf("expected error modal with retry, modal=%q kind=%q", m.modal, m.terminalRetryKind)
	}
	if view := ansi.Strip(m.modalView()); !strings.Contains(view, terminalRetryButton) {
		t.Fatal("retry button not rendered")
	}
	cmd := press(m, "t")
	if cmd == nil || m.modal != "" || m.busy == "" {
		t.Fatal("t must start the terminal retry")
	}
	// tea.Exec is executed by the Bubble Tea runtime; run the same operation
	// the way terminalOperation.Run does.
	run := &terminalOperation{op: m.pushOpForTest(), ctx: domain.WithTerminal(m.ctx)}
	if err := run.Run(); err != nil {
		t.Fatal(err)
	}
	if len(stub.attempts) != 2 || !stub.attempts[1] {
		t.Fatalf("retry must run with terminal access: %v", stub.attempts)
	}
	m.Update(run.msg)
	if m.modal != "result" || m.resultKind != "push" {
		t.Fatalf("retry result not shown: modal=%q", m.modal)
	}
}

func TestUnrelatedErrorDoesNotOfferTerminalRetry(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	m.terminalRetryKind = "push"
	m.Update(actionMsg{id: m.dialogID, kind: "push", err: errors.New("non-fast-forward")})
	if m.terminalRetryKind != "" || strings.Contains(ansi.Strip(m.modalView()), terminalRetryButton) {
		t.Fatal("retry offered for an error that does not need a terminal")
	}
	if cmd := press(m, "t"); cmd != nil {
		t.Fatal("t must do nothing without a retry offer")
	}
	_ = tea.Quit
}

// pushOpForTest builds the retry operation the same way retryInTerminal does.
func (m *Model) pushOpForTest() operation {
	m.busy, m.mutating = "", false
	return m.pushOp()
}
