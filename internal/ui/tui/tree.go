package tui

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

type treeRow struct {
	node  *domain.TreeNode
	depth int
}

func (m *Model) rebuildTree() {
	m.rows = nil
	if m.studio == nil {
		return
	}
	var walk func(*domain.TreeNode, int)
	walk = func(node *domain.TreeNode, depth int) {
		m.rows = append(m.rows, treeRow{node: node, depth: depth})
		if node.IsDir && m.expanded[node.Path] {
			for _, child := range node.Children {
				walk(child, depth+1)
			}
		}
	}
	for _, child := range m.studio.Root.Children {
		walk(child, 0)
	}
	m.selected = min(m.selected, max(0, len(m.rows)-1))
	m.keepTreeVisible()
}

func (m *Model) selectedNode() *domain.TreeNode {
	if m.selected >= 0 && m.selected < len(m.rows) {
		return m.rows[m.selected].node
	}
	return nil
}

func (m *Model) keepTreeVisible() {
	h := m.bodyHeight()
	if m.selected < m.treeOffset {
		m.treeOffset = m.selected
	}
	if m.selected >= m.treeOffset+h {
		m.treeOffset = m.selected - h + 1
	}
	m.treeOffset = min(max(0, m.treeOffset), max(0, len(m.rows)-h))
}

func (m *Model) moveTree(delta int) {
	m.selected = min(max(0, m.selected+delta), max(0, len(m.rows)-1))
	m.keepTreeVisible()
}

func displayName(name string) string {
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return strconv.QuoteToGraphic(name)
	}
	return name
}

func fileIcon(node *domain.TreeNode, open, nerd bool) string {
	if node.IsDir {
		if nerd {
			if open {
				return ""
			}
			return ""
		}
		if open {
			return "▾"
		}
		return "▸"
	}
	if nerd {
		if strings.HasPrefix(node.Name, ".env") {
			return "󰒓"
		}
		switch strings.ToLower(filepath.Ext(node.Name)) {
		case ".go":
			return ""
		case ".md":
			return ""
		case ".json", ".yml", ".yaml", ".toml", ".env":
			return "󰒓"
		}
		return ""
	}
	if strings.HasPrefix(node.Name, ".env") {
		return "[env]"
	}
	switch strings.ToLower(filepath.Ext(node.Name)) {
	case ".go":
		return "[go]"
	case ".md":
		return "[md]"
	case ".json", ".yml", ".yaml", ".toml", ".ini":
		return "[cfg]"
	}
	return "·"
}

func badges(node *domain.TreeNode) string {
	var badges []string
	if node.Status.IsConflicted() {
		badges = append(badges, redStyle.Render("!"))
	}
	if node.Status.IsStaged() || node.StagedCount > 0 {
		badges = append(badges, greenStyle.Render("+"))
	}
	if node.Status.IsUnstaged() || node.UnstagedCount > 0 {
		badges = append(badges, yellowStyle.Render("~"))
	}
	if node.Status.IsUntracked() || node.UntrackedCount > 0 {
		badges = append(badges, cyanStyle.Render("?"))
	}
	if node.Status.IsDeleted() || node.DeletedCount > 0 {
		badges = append(badges, redStyle.Render("-"))
	}
	return strings.Join(badges, "")
}

func (m *Model) treeView(width int) string {
	lines := make([]string, m.bodyHeight())
	for i := range lines {
		index := m.treeOffset + i
		if index >= len(m.rows) {
			lines[i] = fit("", width)
			continue
		}
		row := m.rows[index]
		marker := "  "
		if index == m.selected {
			marker = "› "
		}
		label := marker + strings.Repeat("  ", row.depth) + fileIcon(row.node, m.expanded[row.node.Path], m.nerdFonts) + " " + displayName(row.node.Name)
		badge := badges(row.node)
		label = fit(label, max(1, width-lipgloss.Width(badge)-1)) + " " + badge
		if index == m.selected {
			bg := lipgloss.Color("#252D42")
			if m.focus == "tree" {
				bg = lipgloss.Color("#343052")
			}
			label = lipgloss.NewStyle().Background(bg).Foreground(ink).Render(fit(label, width))
		}
		lines[i] = fit(label, width)
	}
	return strings.Join(lines, "\n")
}
