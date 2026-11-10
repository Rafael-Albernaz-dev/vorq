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
	refreshing := m.studio != nil && len(m.rows) > 0 && m.studio.Workspace.Path == tree.Workspace.Path
	if len(m.rows) > 0 {
		m.rememberTree()
	}
	p := m.positions[m.activeTab]
	offset := m.treeOffset
	reload := m.reloadInventory
	m.reloadInventory = false
	if refreshing && !reload {
		carryInventory(m.studio.Root, tree.Root)
	}
	m.studio = tree
	m.snapshotSummary = tree.Changes.Summary()
	m.stale = false
	m.inventoryCount = tree.Root.FileCount()
	m.dir = tree.Workspace.Path
	m.rebuildTree()
	m.restoreSelection(p)
	m.pendingSelection = treePosition{}
	if node := m.selectedNode(); p.path != "" && (node == nil || node.Path != p.path) {
		// The selected file may live in an expanded folder that is still being
		// read; restore it when that folder arrives instead of jumping.
		m.pendingSelection = p
	}
	if refreshing {
		// A refresh must not move the list the user is looking at: keep the
		// previous scroll position (only clamped to the new length) instead of
		// re-anchoring the viewport on the selection.
		m.treeOffset = min(max(0, offset), max(0, len(m.rows)-m.bodyHeight()))
	}
	commands := []tea.Cmd{m.loadSelected()}
	var walk func(*domain.TreeNode)
	walk = func(n *domain.TreeNode) {
		if n.Lazy && m.expanded["/"+n.Path] {
			commands = append(commands, m.readInventory(n.Path, 0))
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(tree.Root)
	return tea.Batch(commands...)
}

// carryInventory moves already loaded children of lazy (ignored) directories
// from the previous snapshot into the new one, so a refresh neither empties
// expanded folders nor shifts the selection to another file.
func carryInventory(previous, next *domain.TreeNode) {
	if previous == nil || next == nil {
		return
	}
	if next.Lazy && !previous.Lazy && previous.IsDir && len(previous.Children) > 0 {
		next.Children, next.Lazy = previous.Children, false
		return
	}
	old := map[string]*domain.TreeNode{}
	for _, c := range previous.Children {
		old[c.Path] = c
	}
	for _, c := range next.Children {
		if prior := old[c.Path]; prior != nil && prior.IsDir && c.IsDir {
			carryInventory(prior, c)
		}
	}
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
