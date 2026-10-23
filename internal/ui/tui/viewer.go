package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

type viewportPosition struct{ offset, column int }
type viewerState struct {
	path           string
	content        *domain.FileContent
	diff           *domain.FileDiff
	mode           string
	lines          []string
	offset, column int
	loading        bool
	positions      map[string]viewportPosition
}

var hunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func (v *viewerState) setMode(mode string) {
	if v.positions == nil {
		v.positions = map[string]viewportPosition{}
	}
	v.positions[v.mode] = viewportPosition{v.offset, v.column}
	v.mode = mode
	v.rebuild()
	p := v.positions[mode]
	v.offset, v.column = p.offset, p.column
}
func (v *viewerState) rebuild() {
	v.lines = nil
	if v.mode != "arquivo" && v.diff != nil {
		text := v.diff.Unstaged
		if v.mode == "stage" {
			text = v.diff.Staged
		}
		oldLine, newLine := 0, 0
		inHunk := false
		for _, line := range strings.Split(strings.TrimSuffix(safeText(text), "\n"), "\n") {
			if text == "" {
				break
			}
			if hunk := hunkPattern.FindStringSubmatch(line); hunk != nil {
				oldLine, _ = strconv.Atoi(hunk[1])
				newLine, _ = strconv.Atoi(hunk[2])
				inHunk = true
				v.lines = append(v.lines, cyanStyle.Render(line))
				continue
			}
			if strings.HasPrefix(line, "@@") {
				inHunk = false
				v.lines = append(v.lines, cyanStyle.Render(line))
				continue
			}
			prefix := "           "
			style := dimStyle
			if inHunk && strings.HasPrefix(line, "+") {
				prefix = fmt.Sprintf("     %4d │", newLine)
				newLine++
				style = greenStyle
			} else if inHunk && strings.HasPrefix(line, "-") {
				prefix = fmt.Sprintf("%4d      │", oldLine)
				oldLine++
				style = redStyle
			} else if inHunk && strings.HasPrefix(line, " ") {
				prefix = fmt.Sprintf("%4d %4d │", oldLine, newLine)
				oldLine++
				newLine++
				style = textStyle
			} else if strings.HasPrefix(line, "+") {
				style = greenStyle
			} else if strings.HasPrefix(line, "-") {
				style = redStyle
			}
			v.lines = append(v.lines, dimStyle.Render(prefix)+style.Render(line))
		}
		if len(v.lines) == 0 {
			label := "trabalho"
			if v.mode == "stage" {
				label = "stage"
			}
			v.lines = []string{dimStyle.Render("Sem alterações no " + label + " para este arquivo."), dimStyle.Render("Tab abre o conteúdo completo.")}
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
			digits := len(fmt.Sprint(len(lines)))
			for i, line := range lines {
				v.lines = append(v.lines, dimStyle.Render(fmt.Sprintf("%*d | ", digits, i+1))+textStyle.Render(line))
			}
		}
		if len(v.lines) == 0 {
			v.lines = []string{dimStyle.Render("Arquivo vazio")}
		}
		if v.content.Truncated {
			v.lines = append(v.lines, yellowStyle.Render("Conteúdo limitado a 256 KiB."))
		}
	} else {
		v.lines = []string{accentStyle.Render("Mesa de commit"), "", dimStyle.Render("Selecione um arquivo para inspecionar."), dimStyle.Render("e abre a árvore completa do repositório."), "", dimStyle.Render("1 Trabalho · 2 Stage · Tab Arquivo")}
	}
}
func (v *viewerState) scroll(delta, height int) {
	v.offset = min(max(0, v.offset+delta), max(0, len(v.lines)-height))
}
func (v *viewerState) view(width, height int) string {
	lines := make([]string, height)
	for i := range lines {
		line := ""
		if v.loading && i == 0 {
			line = dimStyle.Render("Lendo arquivo…")
		} else if i+v.offset < len(v.lines) {
			line = ansi.Cut(v.lines[i+v.offset], v.column, v.column+width)
		}
		lines[i] = surface(line, width, ColorBg)
	}
	return strings.Join(lines, "\n")
}
