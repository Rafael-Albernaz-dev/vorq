package tui

import (
	"fmt"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

// viewerState is a bounded two-axis viewport; content is formatted only on load
// or mode change, rather than re-reading or re-highlighting it per key press.
type viewerState struct {
	path           string
	content        *domain.FileContent
	diff           *domain.FileDiff
	mode           string
	lines          []string
	offset, column int
	loading        bool
}

func (v *viewerState) rebuild() {
	v.lines = nil
	if v.mode == "diff" && v.diff != nil {
		for _, part := range []struct{ title, text string }{{"STAGED · próximo commit", v.diff.Staged}, {"WORKTREE · fora do stage", v.diff.Unstaged}} {
			if part.text == "" {
				continue
			}
			v.lines = append(v.lines, accentStyle.Render(part.title), "")
			for _, line := range strings.Split(strings.TrimSuffix(safeText(part.text), "\n"), "\n") {
				switch {
				case strings.HasPrefix(line, "@@"):
					line = cyanStyle.Render(line)
				case strings.HasPrefix(line, "+"):
					line = greenStyle.Render(line)
				case strings.HasPrefix(line, "-"):
					line = redStyle.Render(line)
				case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index "):
					line = dimStyle.Render(line)
				}
				v.lines = append(v.lines, line)
			}
			v.lines = append(v.lines, "")
		}
		if len(v.lines) == 0 {
			v.lines = []string{dimStyle.Render("Sem diff para este arquivo. Tab abre o conteúdo.")}
		}
		if v.diff.Truncated {
			v.lines = append(v.lines, yellowStyle.Render("Diff limitado a 256 KiB por seção."))
		}
	} else if v.content != nil {
		if v.content.Notice != "" {
			v.lines = append(v.lines, yellowStyle.Render(safeText(v.content.Notice)), "")
		}
		if !v.content.Binary && v.content.Text != "" {
			lines := strings.Split(strings.TrimSuffix(safeText(v.content.Text), "\n"), "\n")
			width := len(fmt.Sprint(len(lines)))
			for i, line := range lines {
				v.lines = append(v.lines, dimStyle.Render(fmt.Sprintf("%*d │ ", width, i+1))+line)
			}
		}
		if len(v.lines) == 0 {
			v.lines = []string{dimStyle.Render("Arquivo vazio")}
		}
		if v.content.Truncated {
			v.lines = append(v.lines, yellowStyle.Render("Conteúdo limitado a 256 KiB."))
		}
	} else {
		v.lines = []string{accentStyle.Render("Seu workspace, em foco."), "", "Escolha um arquivo na árvore para inspecionar.", "", dimStyle.Render("Enter → abrir   s → stage/unstage   c → commit"), dimStyle.Render("p → push explícito   w → worktrees   ? → ajuda")}
	}
	v.offset, v.column = 0, 0
}

func (v *viewerState) scroll(delta, height int) {
	v.offset = min(max(0, v.offset+delta), max(0, len(v.lines)-height))
}

func (v *viewerState) view(width, height int) string {
	if v.loading {
		return fit(dimStyle.Render("Lendo arquivo…"), width) + strings.Repeat("\n"+fit("", width), max(0, height-1))
	}
	lines := make([]string, height)
	for i := range lines {
		if i+v.offset < len(v.lines) {
			lines[i] = fit(ansi.Cut(v.lines[i+v.offset], v.column, v.column+width), width)
		} else {
			lines[i] = fit("", width)
		}
	}
	return strings.Join(lines, "\n")
}
