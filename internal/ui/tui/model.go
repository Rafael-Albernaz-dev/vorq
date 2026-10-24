package tui

import (
	"context"
	"errors"
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
	id       uint64
	path     string
	content  *domain.FileContent
	diff     *domain.FileDiff
	err      error
	prepared *viewerState
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
	created      bool
	err          error
}
type workspaceMsg struct {
	id   uint64
	tree *domain.WorkspaceTree
	err  error
}
type tickMsg time.Time
type commitDraft struct{ summary, description string }
type workspaceContext struct {
	draft                                                     commitDraft
	activeTab, viewerTab, focusPane, lastDiffTab, viewerGroup string
	expanded                                                  map[string]bool
	positions                                                 map[string]treePosition
	selected, treeOffset                                      int
	viewer                                                    viewerState
}

// Model contains presentation state. All repository reads and mutations run in
// tea.Cmd through app.Service. Each asynchronous read has a freshness token.
type Model struct {
	resultKind, resultBody                         string
	viewerGroup                                    string
	core                                           app.Service
	ctx                                            context.Context
	readCancel, fileCancel, dialogCancel           context.CancelFunc
	dir                                            string
	width, height                                  int
	isWide                                         bool
	activeTab, viewerTab, focusPane, lastDiffTab   string
	studio                                         *domain.WorkspaceTree
	snapshotSummary                                domain.ChangeSummary
	rows                                           []treeRow
	expanded                                       map[string]bool
	positions                                      map[string]treePosition
	selected, treeOffset                           int
	inventoryCount, modalScroll, descriptionOffset int
	viewer                                         viewerState
	loadID, fileID, dialogID, draftRevision        uint64
	loading, mutating, stale                       bool
	busy, notice, modal, lastError                 string
	frame                                          int
	lastRefresh                                    time.Time
	commitSummary, commitDescription               string
	commitField                                    string
	commitEditing                                  bool
	summaryCursor, descriptionCursor               int
	commitPhase                                    string
	reviewDiff, reviewMessage                      string
	commitCount                                    int
	reviewPaths                                    []string
	commitType                                     string
	suggested                                      bool
	contexts                                       map[string]workspaceContext
	themeIndex                                     int
	appearance                                     *appearanceStore
	themeRevision                                  uint64
	iconMode                                       IconMode
	discardPlan                                    *domain.DiscardPlan
	partialApproved                                string
	drafts                                         map[string]commitDraft
	pushTarget                                     *domain.PushTarget
	worktrees                                      []*domain.Workspace
	worktreeIndex                                  int
}

func NewModel(ctx context.Context, core app.Service, dir string) *Model {
	m := &Model{core: core, ctx: ctx, dir: dir, width: 100, height: 30, activeTab: "alteracoes", viewerTab: "trabalho", focusPane: "tree", commitField: "summary", expanded: map[string]bool{}, positions: map[string]treePosition{}, drafts: map[string]commitDraft{}}
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
func (m *Model) refresh() tea.Cmd {
	if m.readCancel != nil {
		m.readCancel()
	}
	if m.fileCancel != nil {
		m.fileCancel()
	}
	m.loadID++
	m.fileID++
	m.loading = true
	id, dir, core := m.loadID, m.dir, m.core
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	m.readCancel = cancel
	return func() tea.Msg {
		defer cancel()
		tree, err := core.GetWorkspaceTree(ctx, dir)
		return treeMsg{id, tree, err}
	}
}
func (m *Model) loadSelected() tea.Cmd {
	if m.fileCancel != nil {
		m.fileCancel()
	}
	m.fileID++
	id := m.fileID
	node := m.selectedNode()
	if node != nil && node.IsDir {
		m.viewerTab = "arquivo"
		m.viewer = viewerState{path: node.Path, mode: "arquivo", directory: node}
		m.viewer.rebuild()
		return nil
	}
	if node == nil {
		m.viewer = viewerState{}
		m.viewer.rebuild()
		return nil
	}
	path, status, dir, core := node.Path, node.Status, m.dir, m.core
	group := ""
	if m.selected >= 0 && m.selected < len(m.rows) {
		group = m.rows[m.selected].group
	}
	if m.viewer.path == path && m.activeTab == "alteracoes" && group != m.viewerGroup {
		if group == "stage" {
			m.setViewerTab("stage")
		} else if group == "work" {
			m.setViewerTab("trabalho")
		}
	}
	if m.viewer.path != path {
		mode := "trabalho"
		switch {
		case group == "stage":
			mode = "stage"
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
	m.viewerGroup = group
	m.viewer.loading = true
	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
	m.fileCancel = cancel
	previous, theme := m.viewer, m.themeName()
	return func() tea.Msg {
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
		v := previous
		v.path, v.theme, v.content, v.diff = path, theme, content, diff
		if err := v.prepareModes(ctx); err != nil {
			return fileMsg{id: id, path: path, err: err}
		}
		return fileMsg{id: id, path: path, content: content, diff: diff, err: diffErr, prepared: &v}
	}
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
func (m *Model) changeSummary() domain.ChangeSummary {
	if m.studio == nil {
		return domain.ChangeSummary{}
	}
	return m.snapshotSummary
}
func (m *Model) draftMessage() string {
	p := domain.CommitProposal{Subject: strings.TrimSpace(m.commitSummary), Body: strings.TrimSpace(m.commitDescription)}
	return p.Message()
}
func (m *Model) stage() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	node := m.selectedNode()
	if node == nil || node.IsDir {
		m.notice = "Selecione um arquivo para stage/unstage."
		return nil
	}
	if !node.Status.IsStaged() && !node.Status.IsUnstaged() && !node.Status.IsUntracked() && !node.Status.IsConflicted() {
		m.notice = "Arquivo sem alterações"
		return nil
	}
	if node.Status.IsStaged() && node.Status.IsUnstaged() && m.partialApproved != node.Path {
		m.modal = "unstage"
		m.modalScroll = 0
		return nil
	}
	m.partialApproved = ""
	m.dialogID++
	id, path, dir, core, ctx := m.dialogID, node.Path, m.dir, m.core, m.ctx
	m.busy = "Atualizando stage…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := core.ToggleStage(ctx, dir, path)
		return actionMsg{id: id, kind: "stage", notice: "Stage atualizado: " + path, err: err}
	}
}
func (m *Model) propose(commitType string) tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.commitType = commitType
	m.dialogID++
	id, revision := m.dialogID, m.draftRevision
	m.commitPhase = "loading"
	m.busy = "Gerando sugestão…"
	core, dir := m.core, m.dir
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		proposal, err := core.ProposeCommit(ctx, dir, commitType)
		return proposalMsg{id: id, revision: revision, proposal: proposal, err: err}
	}
}
func (m *Model) review() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	if m.changeSummary().Conflicts > 0 {
		m.notice = "Resolva os conflitos externamente e prepare a resolução antes de comitar."
		return nil
	}
	if strings.TrimSpace(m.commitSummary) == "" {
		m.notice = "Preencha o Resumo antes de confirmar o commit."
		return nil
	}
	if m.changeSummary().Staged == 0 {
		m.notice = "Nenhum arquivo no stage. Use s para preparar um arquivo."
		return nil
	}
	m.dialogID++
	id, revision, core, dir := m.dialogID, m.draftRevision, m.core, m.dir
	m.modal = "commit"
	m.modalScroll = 0
	m.commitPhase = "loading"
	m.busy = "Revisando arquivos no stage…"
	m.commitEditing = false
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		cs, err := core.GetCommitChanges(ctx, dir)
		return reviewMsg{id: id, revision: revision, changes: cs, err: err}
	}
}
func (m *Model) commit() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir, message, diff, ctx := m.dialogID, m.core, m.dir, m.reviewMessage, m.reviewDiff, m.ctx
	m.busy = "Criando commit local…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := core.CreateReviewedCommit(ctx, dir, message, diff)
		if err != nil {
			return actionMsg{id: id, kind: "commit", err: err, created: commitWasCreated(result, err)}
		}
		return actionMsg{id: id, kind: "commit", notice: fmt.Sprintf("Commit local %s · %s", result.ShortHash, result.Subject)}
	}
}
func (m *Model) inspectPush() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "push"
	m.modalScroll = 0
	m.pushTarget = nil
	m.busy = "Inspecionando destino…"
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		target, err := core.GetPushTarget(ctx, dir)
		return pushTargetMsg{id: id, target: target, err: err}
	}
}
func (m *Model) push() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	if m.pushTarget == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, target, ctx := m.dialogID, m.core, m.dir, *m.pushTarget, m.ctx
	m.busy = "Publicando no destino confirmado…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := core.PushChanges(ctx, dir, domain.PushOptions{Target: target})
		if err != nil {
			return actionMsg{id: id, kind: "push", err: err}
		}
		return actionMsg{id: id, kind: "push", notice: "Push concluído: " + result.Target.Remote + "/" + result.Target.RemoteBranch}
	}
}
func (m *Model) inspectWorktrees() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "worktrees"
	m.modalScroll = 0
	m.worktrees = nil
	m.busy = "Listando worktrees…"
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		worktrees, err := core.ListWorktrees(ctx, dir)
		return worktreesMsg{id: id, worktrees: worktrees, err: err}
	}
}
func (m *Model) canAct() bool {
	if m.loading || m.viewer.loading || m.stale || m.studio == nil || m.busy != "" || m.mutating {
		m.notice = "Ação indisponível · aguarde uma leitura válida; r para atualizar."
		return false
	}
	return true
}
func (m *Model) UpdateTree(tree *domain.WorkspaceTree) tea.Cmd {
	if len(m.rows) > 0 {
		m.rememberTree()
	}
	p := m.positions[m.activeTab]
	m.studio = tree
	m.snapshotSummary = tree.Changes.Summary()
	m.stale = false
	m.inventoryCount = tree.Root.FileCount()
	m.dir = tree.Workspace.Path
	m.rebuildTree()
	m.restoreSelection(p)
	return m.loadSelected()
}
func (m *Model) saveWorkspaceContext() {
	m.rememberTree()
	m.drafts[m.dir] = commitDraft{m.commitSummary, m.commitDescription}
	m.contexts[m.dir] = workspaceContext{draft: m.drafts[m.dir], activeTab: m.activeTab, viewerTab: m.viewerTab, focusPane: m.focusPane, lastDiffTab: m.lastDiffTab, viewerGroup: m.viewerGroup, expanded: m.expanded, positions: m.positions, selected: m.selected, treeOffset: m.treeOffset, viewer: m.viewer}
}
func (m *Model) restoreWorkspaceContext(dir string) {
	c, ok := m.contexts[dir]
	if !ok {
		c = workspaceContext{activeTab: "alteracoes", viewerTab: "trabalho", focusPane: "tree", expanded: map[string]bool{}, positions: map[string]treePosition{}}
	}
	m.commitSummary, m.commitDescription = c.draft.summary, c.draft.description
	m.summaryCursor, m.descriptionCursor = len([]rune(c.draft.summary)), len([]rune(c.draft.description))
	m.activeTab, m.viewerTab, m.focusPane, m.lastDiffTab, m.viewerGroup = c.activeTab, c.viewerTab, c.focusPane, c.lastDiffTab, c.viewerGroup
	m.expanded, m.positions = c.expanded, c.positions
	m.selected, m.treeOffset, m.viewer = c.selected, c.treeOffset, c.viewer
	m.rows = nil
	m.draftRevision++
	m.commitEditing = false
	m.modal = ""
	m.suggested = false
}
func (m *Model) switchWorkspace(dir string) tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.loadID++
	m.fileID++
	m.loading = true
	id, core := m.loadID, m.core
	if m.readCancel != nil {
		m.readCancel()
	}
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	m.readCancel = cancel
	return func() tea.Msg {
		defer cancel()
		tree, err := core.GetWorkspaceTree(ctx, dir)
		return workspaceMsg{id, tree, err}
	}
}

func commitWasCreated(result *domain.CommitResult, err error) bool {
	var outcome *domain.CommitMetadataError
	return (result != nil && result.Hash != "") || errors.As(err, &outcome)
}
