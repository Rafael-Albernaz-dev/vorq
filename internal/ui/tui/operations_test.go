package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCherryPickFromHistoryAndOperationAbort(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	git := func(args ...string) string {
		t.Helper()
		out, err := r.Exec(ctx, dir, args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return strings.TrimSpace(out)
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "-A")
	git("commit", "-m", "chore: base")
	git("switch", "-c", "other")
	write("pick.txt", "pick\n")
	git("add", "pick.txt")
	git("commit", "-m", "feat: pick me")
	write("file.go", "package main // theirs\n")
	git("commit", "-am", "feat: conflicting")
	conflicting := git("rev-parse", "HEAD")
	git("switch", "main")
	write("file.go", "package main // ours\n")
	git("commit", "-am", "feat: ours")
	drive(t, m, m.refresh())

	// C on the history screen applies the selected commit.
	drive(t, m, press(m, "H"))
	m.historyOptions.Ref = "other"
	drive(t, m, m.loadHistory())
	found := false
	for i, l := range m.history.Lines {
		if l.Subject == "feat: pick me" {
			m.historyIndex, found = i, true
		}
	}
	if !found {
		t.Fatalf("commit not in history: %+v", m.history.Lines)
	}
	drive(t, m, press(m, "C"))
	if m.modal != modalCherryPick || m.cherryPickPlan == nil || m.cherryPickPlan.Subject != "feat: pick me" {
		t.Fatalf("modal %q plan %+v notice %q", m.modal, m.cherryPickPlan, m.notice)
	}
	assertFrame(t, m)
	drive(t, m, press(m, "enter"))
	if m.modal != modalResult || m.resultKind != "operation" || !strings.Contains(m.resultBody, "cherry-pick concluído") {
		t.Fatalf("modal %q body %q notice %q", m.modal, m.resultBody, m.notice)
	}
	if git("log", "-1", "--format=%s") != "feat: pick me" {
		t.Fatal("commit not applied")
	}
	drive(t, m, press(m, "enter"))
	before := git("rev-parse", "HEAD")

	// A conflicting pick stops; the operation dialog aborts it.
	m.historyOptions.Ref = "other"
	drive(t, m, m.loadHistory())
	for i, l := range m.history.Lines {
		if l.Hash == conflicting {
			m.historyIndex = i
		}
	}
	drive(t, m, press(m, "C"))
	drive(t, m, press(m, "enter"))
	if !strings.Contains(m.resultBody, "parado para resolução") || !strings.Contains(m.resultBody, "file.go") {
		t.Fatalf("body %q notice %q", m.resultBody, m.notice)
	}
	drive(t, m, press(m, "enter"))
	drive(t, m, press(m, "esc"))
	drive(t, m, m.inspectOperation())
	if m.modal != modalOperation || m.operationState == nil || m.operationState.Kind != "cherry-pick" {
		t.Fatalf("modal %q state %+v", m.modal, m.operationState)
	}
	assertFrame(t, m)
	drive(t, m, press(m, "a"))
	if !strings.Contains(m.resultBody, "cherry-pick abortado") || git("rev-parse", "HEAD") != before {
		t.Fatalf("body %q", m.resultBody)
	}
}
