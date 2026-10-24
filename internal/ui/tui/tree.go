package tui

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

type treeRow struct {
	node    *domain.TreeNode
	depth   int
	group   string
	heading string
	count   int
}
type treePosition struct {
	path, group      string
	selected, offset int
}

func (m *Model) rememberTree() {
	p := treePosition{selected: m.selected, offset: m.treeOffset}
	if m.selected >= 0 && m.selected < len(m.rows) {
		row := m.rows[m.selected]
		p.group = row.group
		if row.node != nil {
			p.path = row.node.Path
		}
	}
	m.positions[m.activeTab] = p
}

func (m *Model) switchTree(tab string) {
	if m.activeTab == tab {
		return
	}
	m.rememberTree()
	m.activeTab = tab
	p := m.positions[tab]
	if p.path == "" {
		p = m.positions[otherTree(tab)]
	}
	if tab == "repositorio" {
		for parent := path.Dir(p.path); parent != "." && parent != ""; parent = path.Dir(parent) {
			m.expanded["/"+parent] = true
		}
	}
	m.rebuildTree()
	m.selected = p.selected
	m.treeOffset = p.offset
	m.restoreSelection(p)
}
func otherTree(tab string) string {
	if tab == "alteracoes" {
		return "repositorio"
	}
	return "alteracoes"
}

func (m *Model) restoreSelection(p treePosition) {
	found := -1
	for i, row := range m.rows {
		if row.node != nil && row.node.Path == p.path {
			if found < 0 {
				found = i
			}
			if row.group == p.group {
				found = i
				break
			}
		}
	}
	if found >= 0 {
		m.selected = found
	}
	m.selected = min(max(0, m.selected), max(0, len(m.rows)-1))
	if len(m.rows) > 0 && m.rows[m.selected].node == nil {
		m.moveTree(1)
	}
	m.keepTreeVisible()
}

func (m *Model) rebuildTree() {
	m.rows = nil
	if m.studio == nil {
		return
	}
	var walk func(*domain.TreeNode, int, string)
	walk = func(node *domain.TreeNode, depth int, group string) {
		m.rows = append(m.rows, treeRow{node: node, depth: depth, group: group})
		if node.IsDir && m.expanded[group+"/"+node.Path] {
			for _, child := range node.Children {
				walk(child, depth+1, group)
			}
		}
	}
	if m.activeTab == "repositorio" {
		for _, node := range m.studio.Root.Children {
			walk(node, 0, "")
		}
	} else {
		groups := []struct {
			key, title string
			files      []domain.FileStatus
		}{{key: "stage", title: "No stage"}, {key: "work", title: "Fora do stage"}, {key: "new", title: "Novos"}, {key: "conflict", title: "Conflitos"}}
		for _, f := range m.studio.Changes.Files {
			if f.IsConflicted() {
				groups[3].files = append(groups[3].files, f)
				continue
			}
			if f.IsStaged() {
				groups[0].files = append(groups[0].files, f)
			}
			if f.IsUnstaged() {
				groups[1].files = append(groups[1].files, f)
			}
			if f.IsUntracked() {
				groups[2].files = append(groups[2].files, f)
			}
		}
		for _, group := range groups {
			if len(group.files) == 0 {
				continue
			}
			m.rows = append(m.rows, treeRow{group: group.key, heading: group.title, count: len(group.files)})
			for _, f := range group.files {
				node := &domain.TreeNode{
					Name:   path.Base(f.Path),
					Path:   f.Path,
					IsDir:  false,
					Status: f,
				}
				m.rows = append(m.rows, treeRow{node: node, depth: 0, group: group.key})
			}
		}
	}
	m.restoreSelection(treePosition{})
}

func (m *Model) selectedNode() *domain.TreeNode {
	if m.selected >= 0 && m.selected < len(m.rows) {
		return m.rows[m.selected].node
	}
	return nil
}
func (m *Model) expandedKey() string {
	if m.selected >= 0 && m.selected < len(m.rows) {
		return m.rows[m.selected].group + "/" + m.rows[m.selected].node.Path
	}
	return ""
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
	if len(m.rows) == 0 {
		return
	}
	index := min(max(0, m.selected+delta), len(m.rows)-1)
	step := 1
	if delta < 0 {
		step = -1
	}
	for index >= 0 && index < len(m.rows) && m.rows[index].node == nil {
		index += step
	}
	if index < 0 || index >= len(m.rows) {
		return
	}
	m.selected = index
	m.keepTreeVisible()
}
func displayName(name string) string {
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return strconv.QuoteToGraphic(name)
	}
	return name
}

func fileIcon(node *domain.TreeNode, open bool) string {
	if node.IsDir {
		if open {
			return "▾ [DIR]"
		}
		return "▸ [DIR]"
	}
	name := strings.ToLower(node.Name)
	ext := strings.ToLower(path.Ext(node.Name))
	token := "[FILE]"
	switch {
	case strings.HasPrefix(name, ".env"):
		token = "[ENV]"
	case name == "go.mod" || name == "go.sum":
		token = "[MOD]"
	case name == ".gitignore" || name == ".gitattributes" || name == ".gitmodules":
		token = "[GIT]"
	case name == "license" || name == "licence" || ext == ".txt":
		token = "[TXT]"
	case ext == ".go":
		token = "[GO]"
	case ext == ".tsx":
		token = "[TSX]"
	case ext == ".jsx":
		token = "[JSX]"
	case ext == ".ts" || ext == ".mts" || ext == ".cts":
		token = "[TS]"
	case ext == ".js" || ext == ".mjs" || ext == ".cjs":
		token = "[JS]"
	case ext == ".py" || ext == ".pyi":
		token = "[PY]"
	case ext == ".html" || ext == ".htm":
		token = "[HTML]"
	case ext == ".css":
		token = "[CSS]"
	case ext == ".md" || ext == ".markdown":
		token = "[MD]"
	case ext == ".json" || ext == ".jsonc":
		token = "[JSON]"
	case ext == ".yaml" || ext == ".yml":
		token = "[YML]"
	case ext == ".sh" || ext == ".bash" || ext == ".zsh":
		token = "[SH]"
	case ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".svg" || ext == ".ico":
		token = "[IMG]"
	case ext == ".toml" || ext == ".ini" || ext == ".conf":
		token = "[ENV]"
	}
	return token
}

func statusBadge(f domain.FileStatus) string {
	if f.IsConflicted() {
		return redStyle.Render("U")
	}
	if f.IsDeleted() {
		return redStyle.Render("D")
	}
	if f.Staged == "R" || f.Unstaged == "R" {
		return purpleStyle.Render("R")
	}
	if f.IsUntracked() {
		return cyanStyle.Render("?")
	}
	if f.Staged == "C" || f.Unstaged == "C" {
		return purpleStyle.Render("C")
	}
	if f.Staged == "T" || f.Unstaged == "T" {
		return yellowStyle.Render("T")
	}
	if f.Unstaged == "M" || f.Staged == "M" {
		return yellowStyle.Render("M")
	}
	if f.IsStaged() {
		return greenStyle.Render("A")
	}
	return dimStyle.Render("·")
}
func stageMarker(f domain.FileStatus) string {
	if f.IsConflicted() {
		return redStyle.Render("[!]")
	}
	if f.IsStaged() && f.IsUnstaged() {
		return yellowStyle.Render("[-]")
	}
	if f.IsStaged() {
		return greenStyle.Render("[x]")
	}
	if f.IsUnstaged() || f.IsUntracked() {
		return dimStyle.Render("[ ]")
	}
	return "   "
}
func (m *Model) treeView(width int) string {
	lines := make([]string, m.bodyHeight())
	for i := range lines {
		index := m.treeOffset + i
		if index >= len(m.rows) {
			text := ""
			if len(m.rows) == 0 && i == 1 {
				switch {
				case m.loading:
					text = "Lendo inventário…"
				case m.studio == nil || m.stale:
					text = "Inventário indisponível · r para atualizar"
				case m.activeTab == "alteracoes":
					text = "✓ Repositório limpo"
				default:
					text = "Repositório sem arquivos"
				}
			}
			lines[i] = surface(text, width, ColorPanel)
			continue
		}
		row := m.rows[index]
		if row.node == nil {
			count := fmt.Sprint(row.count)
			heading := fit(" ▾ "+strings.ToUpper(row.heading), max(0, width-len(count)-1)) + " " + count
			lines[i] = surface(dimStyle.Render(heading), width, ColorPanel)
			continue
		}
		marker := " "
		if index == m.selected {
			marker = "›"
		}
		node := row.node
		icon := nodeIcon(node, m.expanded[row.group+"/"+node.Path], m.currentIconMode())
		var label string
		if node.IsDir {
			label = marker + " " + strings.Repeat("  ", row.depth) + cyanStyle.Render(icon) + " " + displayName(node.Name)
			count := dimStyle.Render(fmt.Sprint(node.FileCount()))
			label = fit(label, max(1, width-lipgloss.Width(count)-1)) + " " + count
		} else {
			iconWidth := 6
			if m.currentIconMode() != IconModeText {
				iconWidth = 2
			}
			label = marker + statusBadge(node.Status) + " " + stageMarker(node.Status) + " " + strings.Repeat("  ", row.depth) + cyanStyle.Render(fit(icon, iconWidth)) + " " + displayName(node.Name)
		}
		bg := ColorPanel
		if index == m.selected {
			bg = ColorSelection
		}
		lines[i] = surface(label, width, bg)
	}
	return strings.Join(lines, "\n")
}
