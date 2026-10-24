package tui

import (
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

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
	m.contexts[m.dir] = workspaceContext{draft: m.drafts[m.dir], activeTab: m.activeTab, viewerTab: m.viewerTab, focusPane: m.focusPane, lastDiffTab: m.lastDiffTab, viewerGroup: m.viewerGroup, expanded: m.expanded, positions: m.positions, selected: m.selected, treeOffset: m.treeOffset, viewer: m.viewer.navigationOnly()}
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
