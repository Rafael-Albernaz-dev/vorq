package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) inspectPull() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.commitEditing = false
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "pull"
	m.modalScroll = 0
	m.pullTarget = nil
	m.busy = "Inspecionando atualização…"
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		target, err := core.GetPullTarget(ctx, dir)
		return pullTargetMsg{id, target, err}
	}
}
func (m *Model) pull() tea.Cmd {
	if !m.canAct() || m.pullTarget == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, target, ctx := m.dialogID, m.core, m.dir, *m.pullTarget, m.ctx
	m.busy = "Buscando e aplicando fast-forward…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		result, err := core.PullChanges(ctx, dir, target)
		if err != nil {
			return actionMsg{id: id, kind: "pull", err: err}
		}
		return actionMsg{id: id, kind: "pull", notice: fmt.Sprintf("Branch %s atualizada de %s/%s.\n%s", target.Branch, target.Remote, target.RemoteBranch, result.Output)}
	}
}
