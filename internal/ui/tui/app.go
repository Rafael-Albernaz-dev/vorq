// Package tui provides the asynchronous terminal workspace studio.
package tui

import (
	"context"
	"io"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// Run always restores the terminal through Bubble Tea's normal shutdown path.
func Run(ctx context.Context, core app.Service, dir string, input io.Reader, output io.Writer, nerdFonts bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := tea.NewProgram(NewModel(ctx, core, dir, nerdFonts),
		tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
