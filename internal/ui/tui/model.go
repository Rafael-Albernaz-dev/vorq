package tui

import (
	"context"
	"fmt"
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
	proposal *domain.CommitProposal
	count    int
	err      error
}
type pushTargetMsg struct {
	target *domain.PushTarget
	err    error
}
type worktreesMsg struct {
	worktrees []*domain.Workspace
	err       error
}
type actionMsg struct {
	notice string
	err    error
}
type tickMsg time.Time

// Model owns presentation state only. Every repository operation is a tea.Cmd
// calling app.Service; responses carry IDs to reject stale asynchronous reads.
type Model struct {
	core                       app.Service
	ctx                        context.Context
	dir                        string
	width, height              int
	nerdFonts                  bool
	studio                     *domain.WorkspaceTree
	rows                       []treeRow
	expanded                   map[string]bool
	selected, treeOffset       int
	focus                      string
	viewer                     viewerState
	loadID, fileID             uint64
	loading                    bool
	busy                       string
	mutating                   bool
	frame                      int
	lastRefresh                time.Time
	notice                     string
	modal                      string
	commitPhase, commitMessage string
	commitCount                int
	edit                       []rune
	cursor                     int
	pushTarget                 *domain.PushTarget
	worktrees                  []*domain.Workspace
	worktreeIndex              int
}

func NewModel(ctx context.Context, core app.Service, dir string, nerdFonts bool) *Model {
	m := &Model{core: core, ctx: ctx, dir: dir, width: 100, height: 30, nerdFonts: nerdFonts, expanded: map[string]bool{}, focus: "tree"}
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
	path, dir, core, parent := node.Path, m.dir, m.core, m.ctx
	if m.viewer.path != path {
		m.viewer = viewerState{path: path, loading: true}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		diff, diffErr := core.GetFileDiff(ctx, dir, path)
		content, contentErr := core.GetFileContent(ctx, dir, path)
		if contentErr != nil {
			content = &domain.FileContent{Path: path, Notice: contentErr.Error()}
		}
		return fileMsg{id, path, content, diff, diffErr}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(1, msg.Width), max(1, msg.Height)
		m.keepTreeVisible()
		m.viewer.scroll(0, m.bodyHeight())
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
		selectedPath := ""
		if node := m.selectedNode(); node != nil {
			selectedPath = node.Path
		}
		if m.studio == nil {
			for _, node := range msg.tree.Root.Children {
				if node.IsDir {
					m.expanded[node.Path] = true
				}
			}
		}
		m.studio = msg.tree
		m.dir = msg.tree.Workspace.Path
		m.rebuildTree()
		for i, row := range m.rows {
			if row.node.Path == selectedPath {
				m.selected = i
				break
			}
		}
		m.keepTreeVisible()
		return m, m.loadSelected()
	case fileMsg:
		if msg.id != m.fileID {
			return m, nil
		}
		mode, offset, column := m.viewer.mode, m.viewer.offset, m.viewer.column
		m.viewer = viewerState{path: msg.path, content: msg.content, diff: msg.diff, mode: "arquivo"}
		if mode == "diff" && msg.diff != nil && msg.diff.Staged == "" && msg.diff.Unstaged == "" {
			m.viewer.mode = "arquivo"
		} else if mode != "" {
			m.viewer.mode = mode
		} else if msg.diff != nil && (msg.diff.Staged != "" || msg.diff.Unstaged != "") {
			m.viewer.mode = "diff"
		}
		if msg.err != nil {
			m.notice = "Diff: " + msg.err.Error()
		}
		m.viewer.rebuild()
		m.viewer.scroll(offset, m.bodyHeight())
		m.viewer.column = column
	case proposalMsg:
		m.busy = ""
		if msg.err != nil {
			m.modal = ""
			m.notice = "Commit: " + msg.err.Error()
			return m, nil
		}
		if m.modal == "commit" {
			m.commitMessage = msg.proposal.Message()
			m.commitCount = msg.count
			m.commitPhase = "review"
		}
	case pushTargetMsg:
		m.busy = ""
		if msg.err != nil {
			m.modal = ""
			m.notice = "Push: " + msg.err.Error()
			return m, nil
		}
		if m.modal == "push" {
			m.pushTarget = msg.target
		}
	case worktreesMsg:
		m.busy = ""
		if msg.err != nil {
			m.modal = ""
			m.notice = "Worktrees: " + msg.err.Error()
			return m, nil
		}
		if m.modal == "worktrees" {
			m.worktrees = msg.worktrees
			m.worktreeIndex = 0
			for i, w := range msg.worktrees {
				if w.Path == m.dir {
					m.worktreeIndex = i
				}
			}
		}
	case actionMsg:
		m.busy = ""
		m.mutating = false
		m.modal = ""
		if msg.err != nil {
			m.notice = "Erro: " + msg.err.Error()
		} else {
			m.notice = msg.notice
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

func (m *Model) stage() tea.Cmd {
	node := m.selectedNode()
	if node == nil || node.IsDir {
		m.notice = "Selecione um arquivo para stage/unstage."
		return nil
	}
	path, dir, core, ctx := node.Path, m.dir, m.core, m.ctx
	m.busy = "Atualizando stage…"
	m.mutating = true
	return func() tea.Msg {
		err := core.ToggleStage(ctx, dir, path)
		return actionMsg{"Stage atualizado: " + path, err}
	}
}

func (m *Model) propose(commitType string) tea.Cmd {
	m.commitPhase = "loading"
	m.busy = "Gerando proposta…"
	core, dir, parent := m.core, m.dir, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		proposal, err := core.ProposeCommit(ctx, dir, commitType)
		if err != nil {
			return proposalMsg{err: err}
		}
		cs, err := core.GetCommitChanges(ctx, dir)
		if err != nil {
			return proposalMsg{err: err}
		}
		return proposalMsg{proposal, cs.StagedCount(), nil}
	}
}

func (m *Model) commit() tea.Cmd {
	m.busy = "Criando commit local…"
	m.mutating = true
	core, dir, message, ctx := m.core, m.dir, m.commitMessage, m.ctx
	return func() tea.Msg {
		result, err := core.CreateCommit(ctx, dir, message)
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{notice: fmt.Sprintf("Commit local %s · %s", result.ShortHash, result.Subject)}
	}
}

func (m *Model) inspectPush() tea.Cmd {
	m.modal = "push"
	m.pushTarget = nil
	m.busy = "Inspecionando destino…"
	core, dir, parent := m.core, m.dir, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		target, err := core.GetPushTarget(ctx, dir)
		return pushTargetMsg{target, err}
	}
}

func (m *Model) push() tea.Cmd {
	if m.pushTarget == nil {
		return nil
	}
	core, dir, target, ctx := m.core, m.dir, *m.pushTarget, m.ctx
	m.busy = "Publicando no destino confirmado…"
	m.mutating = true
	return func() tea.Msg {
		result, err := core.PushChanges(ctx, dir, domain.PushOptions{Target: target})
		if err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{notice: "Push concluído: " + result.Target.Remote + "/" + result.Target.RemoteBranch}
	}
}

func (m *Model) inspectWorktrees() tea.Cmd {
	m.modal = "worktrees"
	m.worktrees = nil
	m.busy = "Listando worktrees…"
	core, dir, parent := m.core, m.dir, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 20*time.Second)
		defer cancel()
		worktrees, err := core.ListWorktrees(ctx, dir)
		return worktreesMsg{worktrees, err}
	}
}
