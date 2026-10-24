// Package tui provides the asynchronous terminal workspace studio.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// Run always restores the terminal through Bubble Tea's normal shutdown path.
func Run(ctx context.Context, core app.Service, dir string, input io.Reader, output io.Writer, modes ...IconMode) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	model := NewModel(ctx, core, dir)
	if store, err := userAppearanceStore(); err == nil {
		model.restoreAppearance(store)
	} else {
		model.notice = "Não foi possível carregar aparência: " + err.Error()
	}
	if len(modes) > 0 {
		model.SetIconMode(modes[0])
	}
	_, err := tea.NewProgram(model,
		tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	if model.appearance != nil && model.themeRevision > 0 {
		if saveErr := model.appearance.save(model.themeName(), model.themeRevision); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("não foi possível salvar o tema: %w", saveErr))
		}
	}
	return err
}
