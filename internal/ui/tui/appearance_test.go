package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"strings"
	"testing"
)

func TestAppearancePaletteChangesOnlySGRAndKeepsTokensLocal(t *testing.T) {
	m := NewModel(nil, nil, "/workspace")
	original := ColorBg
	for i := range studioPalettes {
		m.themeIndex = i
		frame := "\x1b[48;2;26;27;38mtext\x1b[0m literal 48;2;26;27;38"
		got := m.applyTheme(frame)
		if !strings.HasSuffix(got, "literal 48;2;26;27;38") {
			t.Fatal("appearance rewrote repository text")
		}
		if i > 0 && strings.Contains(got, "\x1b[48;2;26;27;38m") {
			t.Fatalf("palette %s kept base background", m.themeName())
		}
		if ColorBg != original {
			t.Fatal("model changed shared design tokens")
		}
	}
}

func TestAppearanceAdaptsTo256ColorOutput(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(previous)
	m := NewModel(nil, nil, "/workspace")
	m.themeIndex = 6
	base := "\x1b[" + termenv.ANSI256.Color("#1a1b26").Sequence(true) + "mbody\x1b[0m"
	got := m.applyTheme(base)
	want := "\x1b[" + termenv.ANSI256.Color(studioPalettes[6].colors[0]).Sequence(true) + "m"
	if !strings.Contains(got, want) {
		t.Fatalf("256-color palette not applied: %q expected %q", got, want)
	}
}
