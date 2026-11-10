package domain

import (
	"context"
	"fmt"
)

// Terminal access policy for native commands (VORQ-012).
//
// The CLI runs in a normal terminal, so Git, SSH and GPG may prompt there. The
// Studio owns the screen: its commands run without terminal access and a
// missing credential becomes a TerminalRequiredError, which the Studio can
// retry after temporarily handing the terminal back to the user.

type terminalKey struct{}

// WithoutTerminal marks native commands started with ctx as non-interactive.
func WithoutTerminal(ctx context.Context) context.Context {
	return context.WithValue(ctx, terminalKey{}, false)
}

// WithTerminal explicitly allows prompts, overriding WithoutTerminal.
func WithTerminal(ctx context.Context) context.Context {
	return context.WithValue(ctx, terminalKey{}, true)
}

// TerminalAllowed reports whether native commands may prompt (default: yes).
func TerminalAllowed(ctx context.Context) bool {
	allowed, ok := ctx.Value(terminalKey{}).(bool)
	return !ok || allowed
}

// TerminalRequiredError means a credential, passphrase or signing PIN was
// needed but prompting was disabled. Nothing was published or committed by the
// failed attempt; the reviewed operation can be retried with terminal access.
type TerminalRequiredError struct {
	Reason string
	Err    error
}

func (e *TerminalRequiredError) Error() string {
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *TerminalRequiredError) Unwrap() error { return e.Err }
