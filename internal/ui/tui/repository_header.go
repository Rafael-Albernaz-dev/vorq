package tui

import (
	"context"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) repositoryHeader() string {
	label, url := "repositório sem link remoto", ""
	if m.studio != nil && m.studio.Repository != nil && m.studio.Repository.URL != "" {
		url = m.studio.Repository.URL
		label = strings.TrimPrefix(url, "https://")
	}
	label = ansi.Truncate("vorq / "+safeText(label), max(1, m.width-2), "…")
	if url != "" {
		label = "\x1b]8;;" + url + "\x1b\\" + accentStyle.Render(label) + "\x1b]8;;\x1b\\"
	}
	return surface(lipgloss.PlaceHorizontal(m.width, lipgloss.Center, label), m.width, ColorRaised)
}
func (m *Model) openRepository() tea.Cmd {
	if m.studio == nil || m.studio.Repository == nil || m.studio.Repository.URL == "" {
		m.notice = "Link remoto indisponível (nenhum remote web ou seleção ambígua)."
		return nil
	}
	url := m.studio.Repository.URL
	ctx := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		e := exec.CommandContext(ctx, "xdg-open", url).Run()
		return workflowDetailMsg{id: 0, err: e}
	}
}
