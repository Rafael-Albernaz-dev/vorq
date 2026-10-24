package tui

import (
	"context"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// Model contains presentation state. All repository reads and mutations run in
// tea.Cmd through app.Service. Each asynchronous read has a freshness token.
type Model struct {
	core app.StudioService
	operationsState
	navigationState
	commitState
	dialogState
	appearanceState
	workspaceState
}

func NewModel(ctx context.Context, core app.StudioService, dir string) *Model {
	m := &Model{core: core,
		operationsState: operationsState{ctx: ctx},
		navigationState: navigationState{activeTab: "alteracoes", viewerTab: "trabalho", focusPane: "tree", expanded: map[string]bool{}, positions: map[string]treePosition{}},
		appearanceState: appearanceState{width: 100, height: 30},
		workspaceState:  workspaceState{dir: dir, drafts: map[string]commitDraft{}},
		commitState:     commitState{commitField: "summary"},
	}
	m.contexts = map[string]workspaceContext{}
	m.commitType = "feat"
	m.SetIconMode(IconModeEmoji)
	m.viewer.rebuild()
	return m
}
func (m *Model) Init() tea.Cmd { return tea.Batch(m.refresh(), tick()) }
func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case appearanceMsg:
		var cmd tea.Cmd
		if msg.next != nil {
			_, cmd = m.Update(msg.next)
		}
		if msg.err != nil && msg.revision == m.themeRevision {
			m.notice = "Tema aplicado nesta sessão, mas não foi possível salvar: " + msg.err.Error()
		}
		return m, cmd
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.isWide = m.width >= 140 && m.height >= 36
		m.keepTreeVisible()
	case tickMsg:
		m.frame++
		if !m.loading && !m.viewer.loading && !m.mutating && m.busy == "" && m.modal == "" && time.Since(m.lastRefresh) > 5*time.Second {
			return m, tea.Batch(m.refresh(), tick())
		}
		return m, tick()
	case workspaceMsg:
		if msg.id != m.loadID {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.showError("Worktree indisponível; contexto anterior preservado", msg.err)
			return m, nil
		}
		m.saveWorkspaceContext()
		m.dir = msg.tree.Workspace.Path
		m.restoreWorkspaceContext(m.dir)
		return m, m.UpdateTree(msg.tree)
	case treeMsg:
		if msg.id != m.loadID {
			return m, nil
		}
		m.loading = false
		m.lastRefresh = time.Now()
		if msg.err != nil {
			m.stale = true
			m.notice = "Snapshot desatualizado · r para tentar novamente: " + msg.err.Error()
			return m, nil
		}
		return m, m.UpdateTree(msg.tree)
	case fileMsg:
		if msg.id != m.fileID {
			return m, nil
		}
		offset, column := m.viewer.offset, m.viewer.column
		if msg.prepared != nil {
			positions := m.viewer.positions
			m.viewer = *msg.prepared
			m.viewer.positions = positions
		}
		m.viewer.content, m.viewer.diff, m.viewer.loading = msg.content, msg.diff, false
		m.viewer.mode = m.viewerTab
		m.viewer.theme = m.themeName()
		m.viewer.rebuild()
		m.viewer.offset, m.viewer.column = offset, column
		if msg.err != nil {
			m.notice = "Diff: " + msg.err.Error()
		}
	case discardPlanMsg:
		if msg.id != m.dialogID {
			return m, nil
		}
		m.busy = ""
		if msg.err != nil {
			m.showError("Descarte bloqueado", msg.err)
			return m, nil
		}
		m.discardPlan = msg.plan
	case proposalMsg:
		if msg.id != m.dialogID {
			return m, nil
		}
		m.busy = ""
		if msg.err != nil {
			m.showError("Sugestão", msg.err)
			return m, nil
		}
		if msg.revision != m.draftRevision {
			m.modal = ""
			m.notice = "O rascunho mudou; solicite a sugestão novamente."
			return m, nil
		}
		m.suggested = true
		m.commitSummary = msg.proposal.Subject
		if m.commitDescription == "" {
			m.commitDescription = msg.proposal.Body
		}
		m.summaryCursor = len([]rune(m.commitSummary))
		m.descriptionCursor = len([]rune(m.commitDescription))
		m.draftRevision++
		m.modal = ""
		m.focusPane = "commit"
		m.commitField = "summary"
		m.commitEditing = false
		m.notice = "Sugestão aplicada ao resumo. Revise antes de confirmar."
	case reviewMsg:
		if msg.id != m.dialogID {
			return m, nil
		}
		m.busy = ""
		if msg.err != nil {
			m.showError("Revisão de commit", msg.err)
			return m, nil
		}
		if msg.revision != m.draftRevision {
			m.modal = ""
			m.notice = "O rascunho mudou. Revise novamente."
			return m, nil
		}
		m.reviewPaths = nil
		for _, f := range msg.changes.Files {
			if f.IsStaged() {
				m.reviewPaths = append(m.reviewPaths, f.Path)
			}
		}
		m.reviewDiff = msg.changes.StagedDiff
		m.reviewMessage = m.draftMessage()
		m.commitCount = msg.changes.Summary().Staged
		m.commitPhase = "review"
	case pushTargetMsg:
		if msg.id != m.dialogID {
			return m, nil
		}
		m.busy = ""
		if msg.err != nil {
			m.showError("Push", msg.err)
			return m, nil
		}
		m.pushTarget = msg.target
	case worktreesMsg:
		if msg.id != m.dialogID {
			return m, nil
		}
		m.busy = ""
		if msg.err != nil {
			m.showError("Worktrees", msg.err)
			return m, nil
		}
		m.worktrees = msg.worktrees
		m.worktreeIndex = 0
		for i, w := range m.worktrees {
			if w.Path == m.dir {
				m.worktreeIndex = i
			}
		}
	case actionMsg:
		if msg.id != m.dialogID {
			return m, nil
		}
		m.busy = ""
		m.mutating = false
		if msg.err != nil && !msg.created {
			m.showError("Falha na operação", msg.err)
		} else {
			m.modal = ""
			m.notice = msg.notice
			if msg.err == nil && (msg.kind == "commit" || msg.kind == "push") {
				m.modal = "result"
				m.resultKind = msg.kind
				m.resultBody = msg.notice
				m.modalScroll = 0
			}
			if msg.err != nil {
				m.showError("Commit criado; leitura do resultado incompleta", msg.err)
			}
			if msg.kind == "commit" {
				m.suggested = false
				m.reviewPaths = nil
				m.commitSummary = ""
				m.commitDescription = ""
				m.summaryCursor = 0
				m.descriptionCursor = 0
				m.draftRevision++
				m.commitEditing = false
				m.commitField = "summary"
			}
		}
		return m, m.refresh()
	case tea.MouseMsg:
		return m.mouse(msg)
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}
func (m *Model) showError(context string, err error) {
	m.lastError = context + "\n\n" + err.Error()
	m.notice = context + ": " + err.Error()
	m.modal = "error"
	m.modalScroll = 0
}
func (m *Model) canAct() bool {
	if m.loading || m.viewer.loading || m.stale || m.studio == nil || m.busy != "" || m.mutating {
		m.notice = "Ação indisponível · aguarde uma leitura válida; r para atualizar."
		return false
	}
	return true
}
