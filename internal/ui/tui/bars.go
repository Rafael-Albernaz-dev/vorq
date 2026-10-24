package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func barSeparator() string { return dimStyle.Foreground(ColorLine).Render("  |  ") }

func (m *Model) contextBar(branch string, ahead, behind int) string {
	syncStyle := dimStyle
	if ahead > 0 {
		syncStyle = greenStyle
	}
	if behind > 0 {
		syncStyle = yellowStyle
	}
	return strings.Join([]string{
		cyanStyle.Bold(true).Render(displayName(branch)),
		syncStyle.Render(fmt.Sprintf("↑%d ↓%d", ahead, behind)),
		accentStyle.Render(m.themeName()),
		dimStyle.Render("ícones: ") + cyanStyle.Render(m.iconModeLabel()),
	}, barSeparator())
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
