package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// VORQ-012 hybrid credentials: the Studio runs native commands without a
// terminal. When an operation fails only because Git, SSH or GPG needed to
// prompt, the error modal offers "Tentar no terminal": the Studio is
// suspended, the same reviewed operation runs with terminal access (the core
// revalidates everything), and the Studio resumes with the result.

const terminalRetryButton = "[Terminal]" // modal buttons cannot contain spaces

type operation func(context.Context) tea.Msg

// offerTerminalRetry remembers which reviewed operation may be retried.
func (m *Model) offerTerminalRetry(kind string, err error) {
	m.terminalRetryKind = ""
	var needs *domain.TerminalRequiredError
	if errors.As(err, &needs) && m.retryBuilder(kind) != nil {
		m.terminalRetryKind = kind
	}
}

// retryBuilder rebuilds the operation from the current reviewed state, so a
// plan resumes from its saved progress instead of an outdated copy.
func (m *Model) retryBuilder(kind string) func() operation {
	switch kind {
	case "push":
		if m.pushTarget != nil {
			return m.pushOp
		}
	case "pull":
		if m.pullTarget != nil {
			return m.pullOp
		}
	case "commit":
		if m.reviewMessage != "" {
			return m.commitOp
		}
	case "execute":
		if m.commitPlan != nil {
			return m.planOp
		}
	case "pr":
		if m.lastPRDraft != nil {
			return m.prOp
		}
	}
	return nil
}

func (m *Model) retryInTerminal() tea.Cmd {
	build := m.retryBuilder(m.terminalRetryKind)
	m.terminalRetryKind = ""
	if build == nil {
		return nil
	}
	m.modal = ""
	if !m.canAct() {
		return nil
	}
	op := build()
	run := &terminalOperation{op: op, ctx: domain.WithTerminal(m.ctx)}
	return tea.Exec(run, func(err error) tea.Msg {
		if err != nil && run.msg == nil {
			return actionMsg{id: m.dialogID, err: fmt.Errorf("não foi possível usar o terminal: %w", err)}
		}
		return run.msg
	})
}

// terminalOperation is a tea.ExecCommand: Bubble Tea releases the terminal
// (leaves the alternate screen and raw mode) around Run, so prompts opened by
// Git/SSH/GPG on the TTY appear in the normal terminal.
type terminalOperation struct {
	op     operation
	ctx    context.Context
	msg    tea.Msg
	stdout io.Writer
}

func (t *terminalOperation) Run() error {
	if t.stdout != nil {
		fmt.Fprintln(t.stdout, "\nvorq · executando no terminal. Git, SSH ou GPG podem pedir credenciais abaixo; o Studio volta ao terminar.")
	}
	t.msg = t.op(t.ctx)
	return nil
}
func (t *terminalOperation) SetStdin(io.Reader)    {}
func (t *terminalOperation) SetStdout(w io.Writer) { t.stdout = w }
func (t *terminalOperation) SetStderr(io.Writer)   {}
