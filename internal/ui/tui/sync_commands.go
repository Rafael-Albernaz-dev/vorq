package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// syncButton sits beside Atualizar: it brings the remote default branch into
// the current branch through a reviewed merge (VORQ-019).
const syncButton = "[⇣ Trazer main]"

type syncTargetMsg struct {
	id     uint64
	target *domain.SyncTarget
	err    error
}

func (m *Model) inspectSync() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.commitEditing = false
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = modalSync
	m.modalScroll = 0
	m.syncTarget = nil
	m.busy = "Inspecionando branch base…"
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		target, err := core.GetSyncTarget(ctx, dir)
		return syncTargetMsg{id, target, err}
	}
}

func (m *Model) receiveSyncTarget(msg syncTargetMsg) tea.Cmd {
	if msg.id != m.dialogID {
		return nil
	}
	m.busy = ""
	if msg.err != nil {
		m.showError("Trazer base bloqueado", msg.err)
		return nil
	}
	if msg.target.OnBase {
		// On the base itself, bringing the base is the regular fast-forward.
		m.modal = ""
		return m.inspectPull()
	}
	m.syncTarget = msg.target
	return nil
}

func (m *Model) syncKey(s string) tea.Cmd {
	t := m.syncTarget
	if t == nil || !m.canAct() {
		return nil
	}
	if t.MergeInProgress {
		if s == "a" {
			return m.abortMerge()
		}
		return nil
	}
	if s == "enter" {
		op, ctx := m.syncOp(), m.ctx
		return func() tea.Msg { return op(ctx) }
	}
	return nil
}

func (m *Model) syncOp() operation {
	m.dialogID++
	id, core, dir, target := m.dialogID, m.core, m.dir, *m.syncTarget
	m.busy = fmt.Sprintf("Buscando %s/%s e fazendo merge…", target.Remote, target.Base)
	m.mutating = true
	return func(ctx context.Context) tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		result, err := core.SyncFromBase(ctx, dir, target)
		if err != nil {
			return actionMsg{id: id, kind: "sync", err: err}
		}
		return actionMsg{id: id, kind: "sync", notice: syncNotice(result)}
	}
}

func (m *Model) abortMerge() tea.Cmd {
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.busy = "Abortando merge…"
	m.mutating = true
	ctx := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := core.AbortMerge(ctx, dir); err != nil {
			return actionMsg{id: id, kind: "sync", err: err}
		}
		return actionMsg{id: id, kind: "sync", notice: "Merge abortado; a branch voltou ao estado anterior."}
	}
}

func syncNotice(r *domain.SyncResult) string {
	t := r.Target
	switch {
	case r.UpToDate:
		return fmt.Sprintf("%s já contém %s/%s; nada a fazer.", t.Branch, t.Remote, t.Base)
	case r.Stopped:
		return "Merge parado para resolução.\n\n" + syncConflictText(r.Conflicts) + "\n\nResolva os arquivos, coloque-os no stage e faça o commit para concluir o merge. Para desistir, use " + syncButton + " de novo e tecle a."
	}
	return fmt.Sprintf("%d commit(s) de %s/%s trazidos para %s.\n%s", r.Incoming, t.Remote, t.Base, t.Branch, strings.TrimSpace(r.Output))
}

func syncConflictText(paths []string) string {
	if len(paths) == 0 {
		return "Nenhum arquivo em conflito; um hook do Git interrompeu o merge."
	}
	lines := []string{fmt.Sprintf("Conflitos (%d):", len(paths))}
	for _, p := range paths {
		lines = append(lines, "  ! "+p)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) syncModal() (title, body, help, buttons string) {
	title = "TRAZER BRANCH BASE · MERGE"
	body = "Inspecionando branch base e estado local…"
	help = "Esc cancelar"
	buttons = "[Voltar]"
	t := m.syncTarget
	if t == nil {
		return
	}
	if t.MergeInProgress {
		title = "MERGE EM ANDAMENTO"
		body = syncConflictText(t.Conflicts) + "\n\nResolva, coloque no stage e faça o commit para concluir.\n\nOu aborte: o merge é desfeito e a branch volta ao estado anterior (resoluções feitas neste merge são descartadas)."
		return title, body, "a abortar merge · Esc voltar", "[Abortar merge] [Voltar]"
	}
	body = fmt.Sprintf("Trazer: %s/%s → Local: %s\n\n↓%d commit(s) conhecidos localmente (a busca pode trazer mais).\n\nBuscar e fazer merge?\n\nSem rebase, stash ou force; o histórico não é reescrito. Conflitos param o merge para você resolver no Studio, com opção de abortar.", t.Remote, t.Base, t.Branch, t.Behind)
	return title, body, "Enter buscar e fazer merge · Esc cancelar", "[Confirmar] [Voltar]"
}
