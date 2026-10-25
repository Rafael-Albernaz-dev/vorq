package tui

import (
	"context"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/clipboard"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

type inventoryMsg struct {
	workspace, path string
	generation      uint64
	offset          int
	nodes           []*domain.TreeNode
	err             error
}
type copiedPathMsg struct {
	path string
	err  error
}

func (m *Model) expandInventory(node *domain.TreeNode) tea.Cmd {
	if node == nil {
		return nil
	}
	if node.More {
		return m.readInventory(node.Path, node.MoreOffset)
	}
	m.expanded[m.expandedKey()] = !m.expanded[m.expandedKey()]
	if node.Lazy && m.expanded[m.expandedKey()] {
		return m.readInventory(node.Path, 0)
	}
	m.rebuildTree()
	return m.loadSelected()
}
func (m *Model) readInventory(path string, offset int) tea.Cmd {
	m.notice = "Carregando pasta…"
	core, dir, id := m.core, m.dir, m.loadID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		defer cancel()
		nodes, err := core.ReadInventoryDirectory(ctx, dir, path, offset)
		return inventoryMsg{workspace: dir, path: path, generation: id, offset: offset, nodes: nodes, err: err}
	}
}
func (m *Model) receiveInventory(msg inventoryMsg) tea.Cmd {
	if msg.workspace != m.dir || msg.generation != m.loadID || m.studio == nil {
		return nil
	}
	if msg.err != nil {
		m.notice = "Inventário: " + msg.err.Error()
		return nil
	}
	var find func(*domain.TreeNode) *domain.TreeNode
	find = func(n *domain.TreeNode) *domain.TreeNode {
		if n.Path == msg.path {
			return n
		}
		for _, c := range n.Children {
			if found := find(c); found != nil {
				return found
			}
		}
		return nil
	}
	node := find(m.studio.Root)
	if node == nil {
		return nil
	}
	if msg.offset == 0 {
		node.Children = msg.nodes
	} else {
		valid := false
		for _, c := range node.Children {
			if c.More && c.MoreOffset == msg.offset {
				valid = true
			}
		}
		if !valid {
			return nil
		}
		var children []*domain.TreeNode
		for _, c := range node.Children {
			if !c.More {
				children = append(children, c)
			}
		}
		node.Children = append(children, msg.nodes...)
	}
	node.Lazy = false
	m.inventoryCount = m.studio.Root.FileCount()
	m.notice = "Pasta carregada; arquivos ignorados não entram em Alterações."
	m.rebuildTree()
	return nil
}
func (m *Model) copyFilePath(path string) tea.Cmd {
	core, dir, ctx := m.core, m.dir, m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		absolute, err := core.AbsoluteFilePath(ctx, dir, path)
		if err == nil {
			err = clipboard.Copy(ctx, absolute)
		}
		return copiedPathMsg{path: absolute, err: err}
	}
}
