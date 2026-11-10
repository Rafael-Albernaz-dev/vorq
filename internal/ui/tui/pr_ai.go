package tui

import (
	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// VORQ-016 Studio flow: in the new-PR form, Ctrl+G or [IA] → provider/model →
// disclosure → the AI drafts title and description, which replace only those
// two fields. The form, its review step and the publish checks stay as they
// are: nothing reaches GitHub without the existing confirmation.

const prAIButton = "[IA]"

type prAIMsg struct {
	id    uint64
	draft *domain.PRDraft
	err   error
}

// prAIModals are the AI dialogs that return to the PR form on Esc.
var prAIModals = map[string]bool{"suggest": true, "ai-models": true, "ai-send": true, "ai-error": true, "ai-options": true}

func (m *Model) syncPRDraftFromForm() {
	if m.formKind != "pr" || len(m.fields) < 3 {
		return
	}
	m.prDraft.Title, m.prDraft.Body, m.prDraft.Base = m.fields[0].Value, m.fields[1].Value, m.fields[2].Value
}

func (m *Model) openPRAI() tea.Cmd {
	if m.formKind != "pr" || m.prDraft.Number != 0 {
		m.notice = "A geração com IA está disponível ao criar um PR."
		return nil
	}
	m.syncPRDraftFromForm()
	m.aiTask = "pr"
	m.aiError = ""
	if m.aiProvider != "" && len(m.aiModels) > 0 {
		m.modal, m.modalScroll = "ai-send", 0
		return nil
	}
	m.modal, m.modalScroll = "suggest", 0
	return nil
}

// backToPRForm restores the form after leaving an AI dialog.
func (m *Model) backToPRForm() bool {
	if m.aiTask != "pr" || m.formKind != "pr" || len(m.fields) == 0 || !prAIModals[m.modal] {
		return false
	}
	m.modal, m.modalScroll, m.busy = "workflow", 0, ""
	return true
}

func (m *Model) generatePRAI() tea.Cmd {
	m.syncPRDraftFromForm()
	m.dialogID++
	id, core, dir, provider, model, base := m.dialogID, m.core, m.dir, m.aiProvider, m.aiModel, m.prDraft.Base
	m.busy = "IA analisando commits e diff da branch…"
	ctx, cancel := m.beginDialog(app.AnalysisTimeout + uiMargin)
	return func() tea.Msg {
		defer cancel()
		d, err := core.DraftPullRequestAI(ctx, dir, base, provider, model)
		return prAIMsg{id: id, draft: d, err: err}
	}
}

func (m *Model) receivePRAI(msg prAIMsg) tea.Cmd {
	if msg.id != m.dialogID {
		return nil
	}
	m.busy = ""
	if msg.err != nil {
		m.aiFailure(msg.err)
		return nil
	}
	if len(m.fields) < 2 || msg.draft == nil {
		return nil
	}
	m.fields[0].Value, m.fields[0].Cursor = msg.draft.Title, len([]rune(msg.draft.Title))
	m.fields[1].Value, m.fields[1].Cursor = msg.draft.Body, 0
	m.prDraft.Title, m.prDraft.Body, m.prDraft.HeadOID = msg.draft.Title, msg.draft.Body, msg.draft.HeadOID
	m.modal, m.modalScroll, m.fieldIndex, m.formReview = "workflow", 0, 0, false
	m.aiTask = ""
	m.notice = "Título e descrição gerados pela IA (" + m.aiProvider + "). Revise e edite antes de enviar; Ctrl+S revisa."
	return nil
}
