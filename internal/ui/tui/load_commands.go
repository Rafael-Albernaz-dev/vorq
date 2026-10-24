package tui

import (
	"context"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

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
