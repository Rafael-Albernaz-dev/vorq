package tui

import (
	"fmt"
	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type aiModelsMsg struct {
	id     uint64
	models []domain.AIModel
	err    error
}

type aiProposalMsg struct {
	id, revision uint64
	proposal     *domain.AIProposal
	err          error
}
type aiApplyMsg struct {
	id, revision uint64
	err          error
}

func (m *Model) chooseAI(key string) tea.Cmd {
	m.aiProvider = providerForKey(key)
	m.aiProposal, m.aiError = nil, ""
	m.aiModels, m.aiModel, m.aiModelIndex = nil, "", 0
	m.aiThenReview = false
	m.modal, m.modalScroll = "ai-models", 0
	m.dialogID++
	id, core, provider := m.dialogID, m.core, m.aiProvider
	m.busy = "Listando modelos de " + provider + "…"
	ctx, cancel := m.beginDialog(app.AIModelsTimeout + uiMargin)
	return func() tea.Msg {
		defer cancel()
		models, err := core.ListAIModels(ctx, provider)
		return aiModelsMsg{id: id, models: models, err: err}
	}
}

func (m *Model) receiveAIModels(msg aiModelsMsg) tea.Cmd {
	if msg.id != m.dialogID {
		return nil
	}
	m.busy = ""
	if msg.err != nil {
		m.aiFailure(msg.err)
		return nil
	}
	if len(msg.models) == 0 {
		m.aiFailure(fmt.Errorf("nenhum modelo disponível"))
		return nil
	}
	m.aiModels = msg.models
	m.aiModelIndex = 0
	m.aiModel = msg.models[0].ID
	return nil
}

func (m *Model) aiModelKey(key string) tea.Cmd {
	if m.busy != "" {
		return nil
	}
	switch key {
	case "up", "k":
		m.aiModelIndex = max(0, m.aiModelIndex-1)
	case "down", "j":
		m.aiModelIndex = min(len(m.aiModels)-1, m.aiModelIndex+1)
	case "enter":
		if len(m.aiModels) > 0 {
			m.aiModel = m.aiModels[m.aiModelIndex].ID
			if m.aiTask == "configure" {
				m.modal = ""
				m.rightTab = "ai"
				m.notice = "Modelo escolhido: " + m.aiModelLabel()
				return nil
			}
			m.modal, m.modalScroll = "ai-send", 0
		}
		return nil
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			index := int(key[0] - '1')
			if index < len(m.aiModels) {
				m.aiModelIndex = index
			}
		}
	}
	if len(m.aiModels) > 0 {
		m.aiModel = m.aiModels[m.aiModelIndex].ID
		m.modalScroll = max(0, m.aiModelIndex+2-m.modalBodyHeight())
	}
	return nil
}

func (m *Model) aiModelRows() []string {
	rows := []string{"Escolha um modelo:"}
	width := max(1, min(90, m.width-6)-4)
	for i, model := range m.aiModels {
		marker := "  "
		if i == m.aiModelIndex {
			marker = "> "
		}
		label := fmt.Sprintf("%s[%d] %s", marker, i+1, model.Name)
		rows = append(rows, ansi.Truncate(safeText(label), width, "…"))
	}
	return rows
}

// providerKeys maps menu keys to providers: c Codex, g Gemini, a Claude.
var providerKeys = map[string]string{"c": "codex", "g": "gemini", "a": "claude"}

func providerForKey(key string) string {
	if p, ok := providerKeys[key]; ok {
		return p
	}
	return "codex"
}

// providerMenu is the provider line shown in every provider chooser.
const providerMenu = "[c] Codex   [g] Gemini (Antigravity)   [a] Claude Code"

// uiMargin lets the core's own deadline (app.*Timeout) fire first, so its
// precise error is shown instead of a generic UI cancellation.
const uiMargin = 15 * time.Second

func (m *Model) aiModelLabel() string {
	if m.aiModel == "" {
		return map[string]string{"codex": "Padrão do Codex", "gemini": "Padrão do Gemini", "claude": "Padrão do Claude Code"}[m.aiProvider]
	}
	return m.aiModel
}

func (m *Model) generateAI() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, revision, dir, provider, model, core := m.dialogID, m.draftRevision, m.dir, m.aiProvider, m.aiModel, m.core
	m.aiProposal = nil
	m.busy = "Analisando staged com " + provider + "…"
	ctx, cancel := m.beginDialog(app.AISuggestTimeout + uiMargin)
	if m.aiTask == "review" || m.aiTask == "plan" {
		cancel()
		return m.generateAutomation()
	}
	return func() tea.Msg {
		defer cancel()
		p, err := core.SuggestCommitAI(ctx, dir, provider, model)
		return aiProposalMsg{id: id, revision: revision, proposal: p, err: err}
	}
}

func (m *Model) receiveAI(msg aiProposalMsg) tea.Cmd {
	if msg.id != m.dialogID {
		return nil
	}
	m.busy = ""
	if msg.revision != m.draftRevision {
		m.aiFailure(fmt.Errorf("o rascunho mudou; gere outra sugestão"))
		return nil
	}
	if msg.err != nil {
		m.aiFailure(msg.err)
		return nil
	}
	if msg.proposal == nil {
		m.aiFailure(fmt.Errorf("resposta vazia"))
		return nil
	}
	m.aiProposal, m.aiRevision = msg.proposal, msg.revision
	m.modal, m.modalScroll = "ai-review", 0
	return nil
}

func (m *Model) aiFailure(err error) {
	m.aiProposal = nil
	m.aiError = err.Error()
	m.modal, m.modalScroll = "ai-error", 0
}

func (m *Model) applyAI() tea.Cmd {
	if !m.canAct() || m.aiProposal == nil {
		return nil
	}
	if m.aiRevision != m.draftRevision {
		m.aiFailure(fmt.Errorf("o rascunho mudou; gere outra sugestão"))
		return nil
	}
	m.dialogID++
	id, revision, core, dir, diff := m.dialogID, m.aiRevision, m.core, m.dir, m.aiProposal.Diff
	m.busy = "Revalidando staged…"
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		return aiApplyMsg{id: id, revision: revision, err: core.ValidateAISuggestion(ctx, dir, diff)}
	}
}

func (m *Model) receiveAIApply(msg aiApplyMsg) tea.Cmd {
	if msg.id != m.dialogID {
		return nil
	}
	m.busy = ""
	if msg.err != nil {
		m.aiFailure(msg.err)
		return nil
	}
	if msg.revision != m.draftRevision || m.aiProposal == nil {
		m.aiFailure(fmt.Errorf("o rascunho mudou; gere outra sugestão"))
		return nil
	}
	m.commitSummary, m.commitDescription = m.aiProposal.Proposal.Subject, m.aiProposal.Proposal.Body
	m.summaryCursor, m.descriptionCursor = len([]rune(m.commitSummary)), len([]rune(m.commitDescription))
	m.descriptionOffset = 0
	m.draftRevision++
	m.suggested = true
	m.suggestionSource = "IA: " + m.aiProvider + " / " + m.aiModelLabel()
	m.aiProposal = nil
	m.modal, m.focusPane, m.commitField = "", "commit", "summary"
	m.commitEditing = false
	m.notice = "Sugestão aplicada. Revise e edite antes de criar o commit local."
	if m.aiThenReview {
		m.aiThenReview = false
		return m.review()
	}
	return nil
}

func (m *Model) aiRecoveryKey(key string) tea.Cmd {
	switch key {
	case "o":
		m.modal, m.modalScroll = "ai-options", 0
	case "r":
		m.modal = "ai-send"
		m.modalScroll = 0
		m.aiProposal = nil
	case "c", "g", "a":
		return m.chooseAI(key)
	case "l":
		m.aiProposal = nil
		m.openSuggestion()
	}
	return nil
}

func (m *Model) aiModelsBody() string { return strings.Join(m.aiModelRows(), "\n") }
