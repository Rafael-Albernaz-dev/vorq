package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestClaudeIsOfferedInEveryProviderMenu(t *testing.T) {
	m, _, _, _, _ := setupStudio(t)
	press(m, "ctrl+g")
	if m.modal != "suggest" || !strings.Contains(ansi.Strip(m.modalView()), "[a] Claude Code") {
		t.Fatalf("suggest menu lacks Claude: %q", ansi.Strip(m.modalView()))
	}
	if cmd := press(m, "a"); cmd == nil || m.aiProvider != "claude" || m.modal != "ai-models" {
		t.Fatalf("a must choose Claude (provider=%q modal=%q)", m.aiProvider, m.modal)
	}
	if m.aiModelLabel() != "Padrão do Claude Code" {
		t.Fatalf("default label %q", m.aiModelLabel())
	}
	m.aiError, m.modal = "boom", "ai-options"
	if !strings.Contains(ansi.Strip(m.modalView()), "[a] Claude Code") {
		t.Fatal("recovery options lack Claude")
	}
}
