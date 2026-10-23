package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	ink         = lipgloss.Color("#DCE4EF")
	muted       = lipgloss.Color("#8693A8")
	accent      = lipgloss.Color("#A78BFA")
	cyan        = lipgloss.Color("#7DD3FC")
	green       = lipgloss.Color("#86EFAC")
	yellow      = lipgloss.Color("#FCD34D")
	red         = lipgloss.Color("#FDA4AF")
	textStyle   = lipgloss.NewStyle().Foreground(ink)
	dimStyle    = lipgloss.NewStyle().Foreground(muted)
	accentStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	greenStyle  = lipgloss.NewStyle().Foreground(green)
	yellowStyle = lipgloss.NewStyle().Foreground(yellow)
	redStyle    = lipgloss.NewStyle().Foreground(red)
	cyanStyle   = lipgloss.NewStyle().Foreground(cyan)
)

// Treat repository text as text, never as terminal escape sequences.
func safeText(s string) string {
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\x1b' {
			return '␛'
		}
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func overlay(base, dialog string, width, height int) string {
	lines := strings.Split(base, "\n")
	box := strings.Split(dialog, "\n")
	w := lipgloss.Width(dialog)
	x, y := max(0, (width-w)/2), max(0, (height-len(box))/2)
	for i, line := range box {
		row := y + i
		if row >= len(lines) {
			break
		}
		lines[row] = fit(ansi.Cut(lines[row], 0, x), x) + line + ansi.Cut(lines[row], x+w, width)
	}
	return strings.Join(lines, "\n")
}
