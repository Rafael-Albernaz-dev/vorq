package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Assembled at run time: no literal credential in the repository.
var tuiAWS = strings.Join([]string{"AKIA", "Z7QWERTYUIOPLKJH"}, "")

func TestModel_GuardBlocksCommitUntilConfirmarTyped(t *testing.T) {
	m, core, r, dir, _ := setupStudio(t)
	writeStudio(t, dir, "file.go", "package main\n\nconst key = \""+tuiAWS+"\"\n")
	drive(t, m, m.refresh())
	selectPath(t, m, "file.go")
	drive(t, m, press(m, "s"))
	press(m, "c")
	press(m, "feat: add key")
	drive(t, m, press(m, "ctrl+s"))
	view := ansi.Strip(m.View())
	if m.modal != modalCommit || !m.reviewGuard.HasBlocking() || !strings.Contains(view, "BLOQUEADO") || strings.Contains(view, tuiAWS) {
		t.Fatalf("review must list masked blocking findings:\n%s", view)
	}
	if cmd := press(m, "enter"); cmd != nil || m.modal != modalGuard || core.commitCalls != 0 {
		t.Fatal("confirming a blocked review must open the typed override, not commit")
	}
	press(m, "q") // typed text, never quits
	press(m, "confirmar")
	if cmd := press(m, "enter"); cmd != nil || core.commitCalls != 0 || m.modal != modalGuard {
		t.Fatal("a wrong word must not commit")
	}
	press(m, "ctrl+u")
	press(m, "esc")
	if m.modal != modalCommit || m.guardOverride {
		t.Fatal("Esc returns to the review without override")
	}
	press(m, "enter")
	press(m, "CONFIRMAR")
	drive(t, m, press(m, "enter"))
	if core.commitCalls != 1 || m.modal != modalResult || !strings.Contains(m.resultBody, "override registrado: 1") {
		t.Fatalf("typed override must commit and be recorded: modal=%s body=%q err=%q", m.modal, m.resultBody, m.lastError)
	}
	out, err := r.Exec(context.Background(), dir, "log", "-1", "--format=%s")
	if err != nil || strings.TrimSpace(out) != "feat: add key" {
		t.Fatalf("commit %q: %v", out, err)
	}
	// The override was for that commit only; the push review checks again.
	press(m, "enter") // close the result
	drive(t, m, press(m, "p"))
	if m.pushTarget == nil || !m.pushTarget.Guard.HasBlocking() || m.guardOverride {
		t.Fatalf("push must re-evaluate unpublished commits: %+v", m.pushTarget)
	}
	press(m, "enter")
	if m.modal != modalGuard || core.pushCalls != 0 || !strings.Contains(m.guardBody(), "git commit --amend") || !strings.Contains(ansi.Strip(m.View()), "CONFIRMAR") {
		t.Fatalf("blocked push must ask for CONFIRMAR and show remediation:\n%s", ansi.Strip(m.View()))
	}
}
