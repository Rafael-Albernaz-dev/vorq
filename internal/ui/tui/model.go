package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

type treeMsg struct {
	id   uint64
	tree *domain.WorkspaceTree
	err  error
}
type fileMsg struct {
	id      uint64
	path    string
	content *domain.FileContent
	diff    *domain.FileDiff
	err     error
}
type proposalMsg struct {
	id, revision uint64
	proposal     *domain.CommitProposal
	err          error
}
type reviewMsg struct {
	id, revision uint64
	changes      *domain.ChangeSet
	err          error
}
type pushTargetMsg struct {
	id     uint64
	target *domain.PushTarget
	err    error
}
type worktreesMsg struct {
	id        uint64
	worktrees []*domain.Workspace
	err       error
}
type actionMsg struct {
	id           uint64
	kind, notice string
	err          error
}
type tickMsg time.Time
type commitDraft struct{ summary, description string }

// Model contains presentation state. All repository reads and mutations run in
// tea.Cmd through app.Service. Each asynchronous read has a freshness token.
type Model struct {
	core                                         app.Service
	ctx                                          context.Context
	dir                                          string
	width, height                                int
	isWide, nerdFonts                            bool
	activeTab, viewerTab, focusPane, lastDiffTab string
	studio                                       *domain.WorkspaceTree
	rows                                         []treeRow
	expanded                                     map[string]bool
	positions                                    map[string]treePosition
	selected, treeOffset                         int
	inventoryCount, modalScroll                  int
	viewer                                       viewerState
	loadID, fileID, dialogID, draftRevision      uint64
	loading, mutating                            bool
	busy, notice, modal, lastError               string
	frame                                        int
	lastRefresh                                  time.Time
	commitSummary, commitDescription             string
	commitField                                  string
	commitEditing                                bool
	summaryCursor, descriptionCursor             int
	commitPhase                                  string
	reviewDiff, reviewMessage                    string
	commitCount                                  int
	drafts                                       map[string]commitDraft
	pushTarget                                   *domain.PushTarget
	worktrees                                    []*domain.Workspace
	worktreeIndex                                int
}

func NewModel(ctx context.Context, core app.Service, dir string, nerdFonts bool) *Model {
	m := &Model{core: core, ctx: ctx, dir: dir, width: 100, height: 30, nerdFonts: nerdFonts, activeTab: "alteracoes", viewerTab: "trabalho", focusPane: "tree", commitField: "summary", expanded: map[string]bool{}, positions: map[string]treePosition{}, drafts: map[string]commitDraft{}}
	m.viewer.rebuild()
	return m
}
func (m *Model) Init() tea.Cmd { return tea.Batch(m.refresh(), tick()) }
func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}
func (m *Model) refresh() tea.Cmd {
	m.loadID++
	m.fileID++
	m.loading = true
	id, dir, core, parent := m.loadID, m.dir, m.core, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		tree, err := core.GetWorkspaceTree(ctx, dir)
		return treeMsg{id, tree, err}
	}
}
func (m *Model) loadSelected() tea.Cmd {
	m.fileID++
	id := m.fileID
	node := m.selectedNode()
	if node == nil || node.IsDir {
		m.viewer = viewerState{}
		m.viewer.rebuild()
		return nil
	}
	path, status, dir, core, parent := node.Path, node.Status, m.dir, m.core, m.ctx
	if m.viewer.path != path {
		mode := "trabalho"
		switch {
		case status.IsConflicted() || status.IsUnstaged():
			mode = "trabalho"
		case status.IsStaged():
			mode = "stage"
		default:
			mode = "arquivo"
		}
		m.viewerTab = mode
		m.viewer = viewerState{path: path, mode: mode, loading: true, positions: map[string]viewportPosition{}}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		diff, diffErr := core.GetFileDiff(ctx, dir, path)
		content, contentErr := core.GetFileContent(ctx, dir, path)
		if contentErr != nil {
			notice := contentErr.Error()
			if status.IsDeleted() {
				notice = "Arquivo excluído · conteúdo indisponível em disco; inspecione o diff."
			}
			content = &domain.FileContent{Path: path, Notice: notice}
		}
		return fileMsg{id, path, content, diff, diffErr}
	}
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.isWide = m.width >= 140 && m.height >= 36
		m.keepTreeVisible()
	case tickMsg:
		m.frame++
		if !m.loading && m.busy == "" && m.modal == "" && time.Since(m.lastRefresh) > 5*time.Second {
			return m, tea.Batch(m.refresh(), tick())
		}
		return m, tick()
	case treeMsg:
		if msg.id != m.loadID {
			return m, nil
		}
		m.loading = false
		m.lastRefresh = time.Now()
		if msg.err != nil {
			m.notice = "Erro: " + msg.err.Error()
			return m, nil
		}
		m.rememberTree()
		p := m.positions[m.activeTab]
		m.studio = msg.tree
		m.inventoryCount = msg.tree.Root.FileCount()
		m.dir = msg.tree.Workspace.Path
		m.rebuildTree()
		m.restoreSelection(p)
		return m, m.loadSelected()
	case fileMsg:
		if msg.id != m.fileID {
			return m, nil
		}
		offset, column := m.viewer.offset, m.viewer.column
		m.viewer.content, m.viewer.diff, m.viewer.loading = msg.content, msg.diff, false
		m.viewer.mode = m.viewerTab
		m.viewer.rebuild()
		m.viewer.offset, m.viewer.column = offset, column
		if msg.err != nil {
			m.notice = "Diff: " + msg.err.Error()
		}
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
		if msg.err != nil {
			m.showError("Falha na operação", msg.err)
		} else {
			m.modal = ""
			m.notice = msg.notice
			if msg.kind == "commit" {
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
		if m.modal != "" {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.viewer.scroll(-3, m.bodyHeight())
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.viewer.scroll(3, m.bodyHeight())
		}
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
func (m *Model) changeSummary() domain.ChangeSummary {
	if m.studio == nil {
		return domain.ChangeSummary{}
	}
	return m.studio.Changes.Summary()
}
func (m *Model) draftMessage() string {
	p := domain.CommitProposal{Subject: strings.TrimSpace(m.commitSummary), Body: strings.TrimSpace(m.commitDescription)}
	return p.Message()
}
func (m *Model) stage() tea.Cmd {
	node := m.selectedNode()
	if node == nil || node.IsDir {
		m.notice = "Selecione um arquivo para stage/unstage."
		return nil
	}
	if !node.Status.IsStaged() && !node.Status.IsUnstaged() && !node.Status.IsUntracked() && !node.Status.IsConflicted() {
		m.notice = "Arquivo sem alterações"
		return nil
	}
	m.dialogID++
	id, path, dir, core, ctx := m.dialogID, node.Path, m.dir, m.core, m.ctx
	m.busy = "Atualizando stage…"
	m.mutating = true
	return func() tea.Msg {
		err := core.ToggleStage(ctx, dir, path)
		return actionMsg{id: id, kind: "stage", notice: "Stage atualizado: " + path, err: err}
	}
}
func (m *Model) propose(commitType string) tea.Cmd {
	m.dialogID++
	id, revision := m.dialogID, m.draftRevision
	m.commitPhase = "loading"
	m.busy = "Gerando sugestão…"
	core, dir, parent := m.core, m.dir, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		proposal, err := core.ProposeCommit(ctx, dir, commitType)
		return proposalMsg{id: id, revision: revision, proposal: proposal, err: err}
	}
}
func (m *Model) review() tea.Cmd {
	if strings.TrimSpace(m.commitSummary) == "" {
		m.notice = "Preencha o Resumo antes de confirmar o commit."
		return nil
	}
	if m.changeSummary().Staged == 0 {
		m.notice = "Nenhum arquivo no stage. Use s para preparar um arquivo."
		return nil
	}
	m.dialogID++
	id, revision, core, dir, parent := m.dialogID, m.draftRevision, m.core, m.dir, m.ctx
	m.modal = "commit"
	m.modalScroll = 0
	m.commitPhase = "loading"
	m.busy = "Revisando arquivos no stage…"
	m.commitEditing = false
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		cs, err := core.GetCommitChanges(ctx, dir)
		return reviewMsg{id: id, revision: revision, changes: cs, err: err}
	}
}
func (m *Model) commit() tea.Cmd {
	m.dialogID++
	id, core, dir, message, diff, ctx := m.dialogID, m.core, m.dir, m.reviewMessage, m.reviewDiff, m.ctx
	m.busy = "Criando commit local…"
	m.mutating = true
	return func() tea.Msg {
		result, err := core.CreateReviewedCommit(ctx, dir, message, diff)
		if err != nil {
			return actionMsg{id: id, kind: "commit", err: err}
		}
		return actionMsg{id: id, kind: "commit", notice: fmt.Sprintf("Commit local %s · %s", result.ShortHash, result.Subject)}
	}
}
func (m *Model) inspectPush() tea.Cmd {
	m.dialogID++
	id, core, dir, parent := m.dialogID, m.core, m.dir, m.ctx
	m.modal = "push"
	m.modalScroll = 0
	m.pushTarget = nil
	m.busy = "Inspecionando destino…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		target, err := core.GetPushTarget(ctx, dir)
		return pushTargetMsg{id: id, target: target, err: err}
	}
}
func (m *Model) push() tea.Cmd {
	if m.pushTarget == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, target, ctx := m.dialogID, m.core, m.dir, *m.pushTarget, m.ctx
	m.busy = "Publicando no destino confirmado…"
	m.mutating = true
	return func() tea.Msg {
		result, err := core.PushChanges(ctx, dir, domain.PushOptions{Target: target})
		if err != nil {
			return actionMsg{id: id, kind: "push", err: err}
		}
		return actionMsg{id: id, kind: "push", notice: "Push concluído: " + result.Target.Remote + "/" + result.Target.RemoteBranch}
	}
}
func (m *Model) inspectWorktrees() tea.Cmd {
	m.dialogID++
	id, core, dir, parent := m.dialogID, m.core, m.dir, m.ctx
	m.modal = "worktrees"
	m.modalScroll = 0
	m.worktrees = nil
	m.busy = "Listando worktrees…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		worktrees, err := core.ListWorktrees(ctx, dir)
		return worktreesMsg{id: id, worktrees: worktrees, err: err}
	}
}
func (m *Model) switchWorkspace(dir string) tea.Cmd {
	m.drafts[m.dir] = commitDraft{m.commitSummary, m.commitDescription}
	draft := m.drafts[dir]
	m.commitSummary, m.commitDescription = draft.summary, draft.description
	m.summaryCursor = len([]rune(draft.summary))
	m.descriptionCursor = len([]rune(draft.description))
	m.draftRevision++
	m.commitEditing = false
	m.dir = dir
	m.modal = ""
	m.studio = nil
	m.rows = nil
	m.expanded = map[string]bool{}
	m.positions = map[string]treePosition{}
	m.selected = 0
	m.treeOffset = 0
	m.viewer = viewerState{}
	m.viewer.rebuild()
	return m.refresh()
}
