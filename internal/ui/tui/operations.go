package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// Cherry-pick (C on the history screen), rebase onto the base and the
// "operation in progress" dialog (continue / abort) — VORQ-020 C.

const (
	modalCherryPick modalID = "cherry-pick"
	modalRebase     modalID = "rebase"
	modalOperation  modalID = "operation"
)

type cherryPickPlanMsg struct {
	id   uint64
	plan *domain.CherryPickPlan
	err  error
}
type rebasePlanMsg struct {
	id   uint64
	plan *domain.RebasePlan
	err  error
}
type operationStateMsg struct {
	id    uint64
	state *domain.OperationState
	err   error
}

func (m *Model) openCherryPick() tea.Cmd {
	if m.screen != "history" || m.history == nil || m.historyIndex >= len(m.history.Lines) || m.busy != "" || m.mutating {
		return nil
	}
	hash := m.history.Lines[m.historyIndex].Hash
	if hash == "" {
		m.notice = "Selecione um commit para aplicar."
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal, m.modalScroll, m.cherryPickPlan = modalCherryPick, 0, nil
	m.busy = "Revisando cherry-pick…"
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		plan, err := core.PrepareCherryPick(ctx, dir, hash)
		return cherryPickPlanMsg{id, plan, err}
	}
}

func (m *Model) inspectRebase() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.commitEditing = false
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal, m.modalScroll, m.rebasePlan = modalRebase, 0, nil
	m.busy = "Revisando rebase…"
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		plan, err := core.PrepareRebase(ctx, dir)
		return rebasePlanMsg{id, plan, err}
	}
}

func (m *Model) inspectOperation() tea.Cmd {
	if m.busy != "" || m.mutating {
		return nil
	}
	m.commitEditing = false
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal, m.modalScroll, m.operationState = modalOperation, 0, nil
	m.busy = "Verificando operação em andamento…"
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		state, err := core.GetOperationState(ctx, dir)
		return operationStateMsg{id, state, err}
	}
}

// receiveOperationMsg handles the three review messages above.
func (m *Model) receiveOperationMsg(msg tea.Msg) {
	var id uint64
	var err error
	switch v := msg.(type) {
	case cherryPickPlanMsg:
		id, err = v.id, v.err
		if id == m.dialogID && err == nil {
			m.cherryPickPlan = v.plan
		}
	case rebasePlanMsg:
		id, err = v.id, v.err
		if id == m.dialogID && err == nil {
			m.rebasePlan = v.plan
		}
	case operationStateMsg:
		id, err = v.id, v.err
		if id == m.dialogID && err == nil {
			m.operationState = v.state
		}
	}
	if id != m.dialogID {
		return
	}
	m.busy = ""
	if err != nil {
		m.showError("Operação Git", err)
	}
}

func (m *Model) operationKey(s string) tea.Cmd {
	if m.busy != "" || m.mutating {
		return nil
	}
	run := func(label string, f func(context.Context) (*domain.OperationResult, error)) tea.Cmd {
		m.dialogID++
		id, parent := m.dialogID, m.ctx
		m.busy, m.mutating = label, true
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
			defer cancel()
			r, err := f(ctx)
			if err != nil {
				return actionMsg{id: id, kind: "operation", err: err}
			}
			return actionMsg{id: id, kind: "operation", notice: operationNotice(r)}
		}
	}
	core, dir := m.core, m.dir
	switch m.modal {
	case modalCherryPick:
		if s == "enter" && m.cherryPickPlan != nil {
			plan := *m.cherryPickPlan
			return run("Aplicando commit…", func(ctx context.Context) (*domain.OperationResult, error) { return core.CherryPick(ctx, dir, plan) })
		}
	case modalRebase:
		if s == "enter" && m.rebasePlan != nil {
			plan := *m.rebasePlan
			return run("Buscando base e fazendo rebase…", func(ctx context.Context) (*domain.OperationResult, error) { return core.Rebase(ctx, dir, plan) })
		}
	case modalOperation:
		st := m.operationState
		if st == nil || st.Kind == "" {
			return nil
		}
		switch s {
		case "c":
			return run("Continuando "+st.Kind+"…", func(ctx context.Context) (*domain.OperationResult, error) { return core.ContinueOperation(ctx, dir) })
		case "a":
			return run("Abortando "+st.Kind+"…", func(ctx context.Context) (*domain.OperationResult, error) {
				kind, err := core.AbortOperation(ctx, dir)
				return &domain.OperationResult{Kind: kind, Output: "aborted"}, err
			})
		}
	}
	return nil
}

func operationNotice(r *domain.OperationResult) string {
	if r.Output == "aborted" {
		return r.Kind + " abortado; a branch voltou ao estado anterior."
	}
	if r.Stopped {
		return r.Kind + " parado para resolução.\n\n" + syncConflictText(r.Conflicts) + "\n\nResolva os arquivos e coloque-os no stage; depois Git → Operação em andamento → c continuar (ou a abortar)."
	}
	return r.Kind + " concluído.\n" + strings.TrimSpace(r.Output)
}

func (m *Model) operationModal() (title, body, help, buttons string) {
	help, buttons = "Esc voltar", "[Voltar]"
	switch m.modal {
	case modalCherryPick:
		title, body = "CHERRY-PICK", "Revisando…"
		if p := m.cherryPickPlan; p != nil {
			body = fmt.Sprintf("Aplicar: %s %s\nEm: %s (HEAD %s)\n\nO commit novo registra a origem (-x). Requer workspace limpo; conflitos param para você resolver, com opção de abortar.", shortHash(p.Commit), safeText(p.Subject), safeText(p.Branch), shortHash(p.HeadHash))
			help, buttons = "Enter aplicar · Esc cancelar", "[Confirmar] [Voltar]"
		}
	case modalRebase:
		title, body = "REBASE SOBRE A BASE", "Revisando…"
		if p := m.rebasePlan; p != nil {
			body = fmt.Sprintf("Rebase de %s sobre %s/%s\n\n%d commit(s) locais reaplicados · ↓%d conhecidos localmente (a busca pode trazer mais).\n\nSó commits ainda não publicados podem ser reescritos, então nenhum force push será necessário. Conflitos param para você resolver, com opção de abortar.", safeText(p.Branch), safeText(p.Remote), safeText(p.Base), p.Commits, p.Behind)
			help, buttons = "Enter buscar e fazer rebase · Esc cancelar", "[Confirmar] [Voltar]"
		}
	case modalOperation:
		title, body = "OPERAÇÃO EM ANDAMENTO", "Verificando…"
		if st := m.operationState; st != nil {
			if st.Kind == "" {
				body = "Nenhum merge, cherry-pick ou rebase em andamento."
				return
			}
			title = "OPERAÇÃO EM ANDAMENTO · " + strings.ToUpper(st.Kind)
			body = syncConflictText(st.Conflicts) + "\n\nc continuar: depois de resolver e colocar tudo no stage.\na abortar: desfaz a operação e volta ao estado anterior (resoluções feitas nela são descartadas)."
			help, buttons = "c continuar · a abortar · Esc voltar", "[Continuar] [Abortar] [Voltar]"
		}
	}
	return
}
