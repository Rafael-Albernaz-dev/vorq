package tui

import (
	"context"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

type discardPlanMsg struct {
	id   uint64
	plan *domain.DiscardPlan
	err  error
}

func (m *Model) stageAll(unstage bool) tea.Cmd {
	if !m.canAct() {
		return nil
	}
	s := m.changeSummary()
	if unstage && s.Staged+s.Conflicts == 0 {
		m.notice = "Nenhum arquivo no stage"
		return nil
	}
	if !unstage && s.Unstaged+s.Untracked+s.Conflicts == 0 {
		m.notice = "Nenhuma alteração fora do stage"
		return nil
	}
	m.dialogID++
	id, core, dir, parent := m.dialogID, m.core, m.dir, m.ctx
	m.busy, m.mutating = "Preparando todos os arquivos…", true
	if unstage {
		m.busy = "Retirando todos os arquivos do stage…"
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		var err error
		notice := "Todos os arquivos preparados no stage"
		if unstage {
			err = core.UnstageFiles(ctx, dir, []string{"."})
			notice = "Stage limpo; arquivos preservados"
		} else {
			err = core.StageFiles(ctx, dir, []string{"."})
		}
		return actionMsg{id: id, kind: "stage", notice: notice, err: err}
	}
}

func (m *Model) inspectDiscard() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	node := m.selectedNode()
	if node == nil || node.IsDir {
		m.notice = "Selecione um arquivo para descartar."
		return nil
	}
	m.dialogID++
	id, core, dir, path := m.dialogID, m.core, m.dir, node.Path
	ctx, cancel := m.beginDialog(20 * time.Second)
	m.modal, m.modalScroll, m.discardPlan = "discard", 0, nil
	m.busy = "Verificando impacto do descarte…"
	return func() tea.Msg {
		defer cancel()
		plan, err := core.PrepareDiscard(ctx, dir, path)
		return discardPlanMsg{id: id, plan: plan, err: err}
	}
}

func (m *Model) discard(preserve bool) tea.Cmd {
	if m.modal != "discard" || m.discardPlan == nil || !m.canAct() {
		return nil
	}
	plan := *m.discardPlan
	if preserve && !(plan.Delete && plan.Staged) {
		return nil
	}
	m.dialogID++
	id, core, dir, parent := m.dialogID, m.core, m.dir, m.ctx
	m.busy, m.mutating = "Descartando arquivo confirmado…", true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		var err error
		notice := "Descarte concluído: " + plan.Path
		if preserve {
			err = core.UnstageFiles(ctx, dir, []string{"./" + plan.Path})
			notice = "Arquivo preservado e retirado do stage: " + plan.Path
		} else {
			err = core.DiscardFile(ctx, dir, plan, true)
		}
		return actionMsg{id: id, kind: "discard", notice: notice, err: err}
	}
}

// Owned by the update loop; commands capture the context/cancel, never Model.
func (m *Model) beginDialog(timeout time.Duration) (context.Context, context.CancelFunc) {
	if m.dialogCancel != nil {
		m.dialogCancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	m.dialogCancel = cancel
	return ctx, cancel
}
