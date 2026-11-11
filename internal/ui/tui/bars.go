package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func barSeparator() string { return dimStyle.Foreground(ColorLine).Render("  |  ") }

// refreshButton is the explicit workspace reload (VORQ-011): vorq never
// re-reads on a timer, so this button and r are the way to see external edits.
const refreshButton = "[↻ Atualizar]"

// contextRight is the right segment of the context row, shared by rendering
// and mouse hit testing so both always agree on positions.
func (m *Model) contextRight() string {
	right := "Carregando · ↑… ↓… · " + m.themeName()
	if m.studio != nil {
		right = m.contextBar(m.studio.Workspace.Branch, m.studio.Changes.Ahead, m.studio.Changes.Behind)
	}
	if m.stale {
		right += " · DESATUALIZADO"
	}
	right += barSeparator()
	// Below 80 columns only Atualizar fits; Trazer main stays in the Git panel.
	if m.width >= 80 {
		right += accentStyle.Bold(true).Render(syncButton) + "  "
	}
	return right + accentStyle.Bold(true).Render(refreshButton)
}

func (m *Model) contextBar(branch string, ahead, behind int) string {
	syncStyle := dimStyle
	if ahead > 0 {
		syncStyle = greenStyle
	}
	if behind > 0 {
		syncStyle = yellowStyle
	}
	parts := []string{
		cyanStyle.Bold(true).Render(displayName(branch)),
		syncStyle.Render(fmt.Sprintf("↑%d ↓%d", ahead, behind)),
	}
	// Narrow terminals keep branch, sync arrows and the refresh button visible.
	if m.width >= 110 {
		parts = append(parts, accentStyle.Render(m.themeName()), dimStyle.Render("ícones: ")+cyanStyle.Render(m.iconModeLabel()))
	}
	return strings.Join(parts, barSeparator())
}

func (m *Model) summaryBar() string {
	s := m.changeSummary()
	total := greenStyle.Render("✓ 0 alterados")
	if s.Total > 0 {
		total = yellowStyle.Bold(true).Render(fmt.Sprintf("%d alterados", s.Total))
	}
	metric := func(count int, label string, style lipgloss.Style) string {
		if count == 0 {
			style = dimStyle
		}
		return style.Render(fmt.Sprintf(label, count))
	}
	conflictLabel := "!%d conflitos"
	if s.Conflicts > 0 {
		conflictLabel = "!%d CONFLITOS"
	}
	labels := []string{"+%d no stage", "~%d fora", "?%d novos", conflictLabel, "[-%d] em ambos"}
	sep := barSeparator()
	if m.width < 110 {
		labels = []string{"+%d stage", "~%d fora", "?%d novos", conflictLabel, "[-%d] ambos"}
		sep = dimStyle.Foreground(ColorLine).Render(" | ")
	}
	return " " + strings.Join([]string{total,
		metric(s.Staged, labels[0], greenStyle.Bold(true)),
		metric(s.Unstaged, labels[1], yellowStyle),
		metric(s.Untracked, labels[2], cyanStyle),
		metric(s.Conflicts, labels[3], redStyle.Bold(true)),
		metric(s.Both, labels[4], purpleStyle),
	}, sep)
}
