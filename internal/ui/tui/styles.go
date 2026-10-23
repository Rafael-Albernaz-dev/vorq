package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	ColorBg          = lipgloss.Color("#1a1b26")
	ColorPanel       = lipgloss.Color("#16161e")
	ColorRaised      = lipgloss.Color("#24283b")
	ColorText        = lipgloss.Color("#c0caf5")
	ColorMuted       = lipgloss.Color("#9aa5ce")
	ColorLine        = lipgloss.Color("#414868")
	ColorAccent      = lipgloss.Color("#7aa2f7")
	ColorSelection   = lipgloss.Color("#283457")
	ColorGreen       = lipgloss.Color("#9ece6a")
	ColorRed         = lipgloss.Color("#f7768e")
	ColorYellow      = lipgloss.Color("#e0af68")
	ColorPurple      = lipgloss.Color("#bb9af7")
	ColorBar         = lipgloss.Color("#2e4270")
	ColorBarText     = lipgloss.Color("#ffffff")
	ColorCyan        = lipgloss.Color("#7dcfff")
	textStyle        = lipgloss.NewStyle().Foreground(ColorText)
	dimStyle         = lipgloss.NewStyle().Foreground(ColorMuted)
	accentStyle      = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true)
	greenStyle       = lipgloss.NewStyle().Foreground(ColorGreen)
	yellowStyle      = lipgloss.NewStyle().Foreground(ColorYellow)
	redStyle         = lipgloss.NewStyle().Foreground(ColorRed)
	cyanStyle        = lipgloss.NewStyle().Foreground(ColorCyan)
	purpleStyle      = lipgloss.NewStyle().Foreground(ColorPurple)
	activeTabStyle   = lipgloss.NewStyle().Foreground(ColorAccent).Background(ColorRaised).Bold(true)
	inactiveTabStyle = lipgloss.NewStyle().Foreground(ColorMuted).Background(ColorPanel)
)

func surface(s string, width int, bg lipgloss.Color) string {
	style := lipgloss.NewStyle().Foreground(ColorText).Background(bg)
	sample := style.Render(" ")
	prefix := ""
	if end := strings.IndexByte(sample, ' '); end > 0 {
		prefix = sample[:end]
	}
	if prefix == "" {
		return fit(s, width)
	} // Respect NO_COLOR and plain terminals.
	// Foreground fragments reset SGR at their end. Restore this surface's
	// background after each reset so every cell retains the panel color.
	content := strings.ReplaceAll(fit(s, width), "\x1b[0m", "\x1b[0m"+prefix)
	return prefix + content + "\x1b[0m"
}

func tabLabel(label string, active bool) string {
	if active {
		return activeTabStyle.Render("[" + label + "]")
	}
	return inactiveTabStyle.Render(" " + label + " ")
}

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
