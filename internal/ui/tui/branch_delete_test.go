package tui

import (
	"context"
	"strings"
	"testing"
)

func selectBranch(t *testing.T, m *Model, name string) {
	t.Helper()
	for i, b := range m.localBranches {
		if b.Name == name {
			m.branchIndex = i
			return
		}
	}
	t.Fatalf("branch %s not listed", name)
}

func TestBranchDeleteFromSelector(t *testing.T) {
	m, _, r, dir, _ := setupStudio(t)
	ctx := context.Background()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "chore: base"}, {"push", "-u", "origin", "main"}, {"branch", "done"}, {"push", "-u", "origin", "done"}, {"switch", "-c", "wip"}, {"commit", "--allow-empty", "-m", "wip"}, {"switch", "main"}} {
		if _, err := r.Exec(ctx, dir, args...); err != nil {
			t.Fatal(args, err)
		}
	}
	drive(t, m, m.refresh())
	// Merged branch: Enter deletes local and remote.
	drive(t, m, m.inspectBranches())
	selectBranch(t, m, "done")
	drive(t, m, press(m, "d"))
	if m.modal != modalBranchDelete || m.branchDeletePlan == nil || !m.branchDeletePlan.Merged || m.branchDeletePlan.RemoteHash == "" {
		t.Fatalf("modal %q plan %+v notice %q", m.modal, m.branchDeletePlan, m.notice)
	}
	assertFrame(t, m)
	drive(t, m, press(m, "enter"))
	if m.modal != modalResult || !strings.Contains(m.resultBody, "Também apagada em origin") {
		t.Fatalf("modal %q body %q notice %q", m.modal, m.resultBody, m.notice)
	}
	// Unmerged branch: needs typed CONFIRMAR; q is text, not quit.
	drive(t, m, press(m, "enter"))
	drive(t, m, m.inspectBranches())
	selectBranch(t, m, "wip")
	drive(t, m, press(m, "d"))
	if m.branchDeletePlan == nil || !m.branchDeletePlan.NeedsConfirm() {
		t.Fatalf("plan %+v", m.branchDeletePlan)
	}
	drive(t, m, press(m, "q"))
	drive(t, m, press(m, "enter"))
	if m.modal != modalBranchDelete {
		t.Fatalf("wrong word deleted: modal %q", m.modal)
	}
	drive(t, m, press(m, "ctrl+u"))
	for _, c := range "CONFIRMAR" {
		drive(t, m, press(m, string(c)))
	}
	drive(t, m, press(m, "enter"))
	if m.modal != modalResult || !strings.Contains(m.resultBody, "Recuperar: git branch wip") {
		t.Fatalf("modal %q body %q notice %q", m.modal, m.resultBody, m.notice)
	}
	if out, _ := r.Exec(ctx, dir, "branch", "--list", "wip", "done"); strings.TrimSpace(out) != "" {
		t.Fatalf("branches kept: %q", out)
	}
}
