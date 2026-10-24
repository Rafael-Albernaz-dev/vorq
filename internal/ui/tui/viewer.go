package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type viewportPosition struct{ offset, column int }
type renderedMode struct {
	lines            []string
	backgrounds      []lipgloss.Color
	added, deleted   int
	binary, combined bool
}
type viewerState struct {
	path                    string
	content                 *domain.FileContent
	directory               *domain.TreeNode
	diff                    *domain.FileDiff
	mode                    string
	theme                   string
	renderContext           context.Context
	renderCache             map[string]renderedMode
	renderTheme, renderPath string
	renderContent           *domain.FileContent
	renderDiff              *domain.FileDiff
	lines                   []string
	offset, column          int
	loading                 bool
	positions               map[string]viewportPosition
	added, deleted          int
	binary                  bool
	combined                bool
	lineBackgrounds         []lipgloss.Color
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
func (v *viewerState) appendLine(line string, bg lipgloss.Color) {
	v.lines = append(v.lines, line)
	v.lineBackgrounds = append(v.lineBackgrounds, bg)
}

func (v *viewerState) rebuild() {
	cacheable := v.directory == nil && (v.content != nil || v.diff != nil)
	sameContent := v.content == v.renderContent || (v.content != nil && v.renderContent != nil && *v.content == *v.renderContent)
	sameDiff := v.diff == v.renderDiff || (v.diff != nil && v.renderDiff != nil && *v.diff == *v.renderDiff)
	if !sameContent || !sameDiff || v.theme != v.renderTheme || v.path != v.renderPath {
		v.renderCache = nil
	}
	if cached, ok := v.renderCache[v.mode]; cacheable && ok {
		v.lines, v.lineBackgrounds = cached.lines, cached.backgrounds
		v.added, v.deleted, v.binary, v.combined = cached.added, cached.deleted, cached.binary, cached.combined
		return
	}
	v.rebuildContent()
	if cacheable {
		if v.renderCache == nil {
			v.renderCache = map[string]renderedMode{}
		}
		v.renderCache[v.mode] = renderedMode{v.lines, v.lineBackgrounds, v.added, v.deleted, v.binary, v.combined}
		v.renderTheme, v.renderPath, v.renderContent, v.renderDiff = v.theme, v.path, v.content, v.diff
	}
}

// Prepared exclusively inside a tea.Cmd. Clone the map before adding entries
// so an old command and the model never write the same cache concurrently.
func (v *viewerState) prepareModes(ctx context.Context) error {
	cache := make(map[string]renderedMode, 3)
	for mode, rendered := range v.renderCache {
		cache[mode] = rendered
	}
	v.renderCache = cache
	v.renderContext = ctx
	original := v.mode
	for _, mode := range []string{"trabalho", "stage", "arquivo"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		v.mode = mode
		v.rebuild()
	}
	v.mode = original
	v.rebuild()
	v.renderContext = nil
	return ctx.Err()
}

func (v *viewerState) rebuildContent() {
	v.lines, v.lineBackgrounds = nil, nil
	v.added, v.deleted, v.binary, v.combined = 0, 0, false, false
	if v.directory != nil {
		files := []domain.FileStatus{}
		var collect func(*domain.TreeNode)
		collect = func(node *domain.TreeNode) {
			if node.IsDir {
				for _, child := range node.Children {
					collect(child)
				}
				return
			}
			f := node.Status
			f.Path = node.Path
			if f.IsStaged() || f.IsUnstaged() || f.IsUntracked() || f.IsConflicted() {
				files = append(files, f)
			}
		}
		collect(v.directory)
		summary := (&domain.ChangeSet{Files: files}).Summary()
		total := v.directory.FileCount()
		for _, line := range []string{
			textStyle.Render(safeText(v.directory.Path) + "/"),
			dimStyle.Render("Pasta selecionada · conteúdo do repositório"), "",
			fmt.Sprintf("Caminhos descendentes  %d", total),
			fmt.Sprintf("Sem alterações         %d", total-summary.Total),
			fmt.Sprintf("Alterados              %d", summary.Total),
			fmt.Sprintf("No stage               %d", summary.Staged), "",
			dimStyle.Render("Enter ou Espaço expande/recolhe."),
			dimStyle.Render("Selecione um arquivo para ler o conteúdo."),
			dimStyle.Render("Stage permanece uma ação por arquivo."),
		} {
			v.appendLine(line, ColorBg)
		}
		return
	}
	if v.mode != "arquivo" && v.diff != nil {
		text := v.diff.Unstaged
		if v.mode == "stage" {
			text = v.diff.Staged
		}
		v.binary = strings.Contains(text, "Binary files ") || strings.Contains(text, "GIT binary patch")
		if v.binary {
			v.appendLine(yellowStyle.Render("Arquivo binário · sem prévia textual ou contagem de linhas."), ColorBg)
			return
		}
		oldLine, newLine := 0, 0
		ctx := v.renderContext
		if ctx == nil {
			ctx = context.Background()
		}
		highlighted := highlightDiffLinesContext(ctx, v.path, text, v.theme)
		inHunk, combined := false, false
		for row, line := range strings.Split(strings.TrimSuffix(safeText(text), "\n"), "\n") {
			if text == "" {
				break
			}
			if hunk := hunkPattern.FindStringSubmatch(line); hunk != nil {
				oldLine, _ = strconv.Atoi(hunk[1])
				newLine, _ = strconv.Atoi(hunk[2])
				inHunk, combined = true, false
				v.appendLine("       "+cyanStyle.Render(line), ColorPanel)
				continue
			}
			if strings.HasPrefix(line, "@@") {
				inHunk, combined = false, true
				v.combined = true
				v.appendLine("       "+cyanStyle.Render(line), ColorPanel)
				continue
			}
			// Git metadata is represented by the breadcrumb/comparison header.
			// Keep only actual hunk content in the code viewport.
			if !inHunk {
				if combined && len(line) >= 2 && strings.ContainsRune(" +-", rune(line[0])) && strings.ContainsRune(" +-", rune(line[1])) {
					// Combined merge diff: preserve Git's two-parent markers without
					// fabricating line numbers or treating it as a normal patch.
					v.appendLine("       "+textStyle.Render(line), ColorBg)
				}
				continue
			}
			if strings.HasPrefix(line, "\\ No newline") {
				v.appendLine("       "+dimStyle.Render(line), ColorBg)
				continue
			}
			if len(line) == 0 {
				continue
			}
			sign, number, style, bg := line[:1], newLine, textStyle, ColorBg
			switch sign {
			case "+":
				newLine++
				v.added++
				style, bg = greenStyle, ColorAddedBg
			case "-":
				number = oldLine
				oldLine++
				v.deleted++
				style, bg = redStyle, ColorDeletedBg
			case " ":
				oldLine++
				newLine++
			default:
				v.appendLine("       "+dimStyle.Render(line), ColorBg)
				continue
			}
			code, ok := highlighted[row]
			if !ok {
				code = textStyle.Render(line[1:])
			}
			v.appendLine(dimStyle.Render(fmt.Sprintf("%4d ", number))+style.Render(sign+" ")+code, bg)
		}
		if len(v.lines) == 0 {
			label := "trabalho"
			if v.mode == "stage" {
				label = "stage"
			}
			v.appendLine(dimStyle.Render("Sem alterações no "+label+" para este arquivo."), ColorBg)
			v.appendLine(dimStyle.Render("Tab abre o conteúdo completo."), ColorBg)
		}
		if v.diff.Truncated {
			v.appendLine(yellowStyle.Render("Diff limitado a 256 KiB por seção."), ColorBg)
		}
	} else if v.content != nil {
		v.binary = v.content.Binary
		if v.content.Notice != "" {
			v.appendLine(yellowStyle.Render(safeText(v.content.Notice)), ColorBg)
			v.appendLine("", ColorBg)
		}
		if !v.content.Binary && v.content.Text != "" {
			ctx := v.renderContext
			if ctx == nil {
				ctx = context.Background()
			}
			lines := highlightCodeLinesContext(ctx, v.path, v.content.Text, v.theme)
			digits := max(4, len(fmt.Sprint(len(lines))))
			for i, line := range lines {
				v.appendLine(dimStyle.Render(fmt.Sprintf("%*d | ", digits, i+1))+line, ColorBg)
			}
		}
		if len(v.lines) == 0 {
			label := "Arquivo vazio"
			if v.binary {
				label = "Arquivo binário · sem prévia textual ou contagem de linhas."
			}
			v.appendLine(dimStyle.Render(label), ColorBg)
		}
		if v.content.Truncated {
			v.appendLine(yellowStyle.Render("Conteúdo limitado a 256 KiB."), ColorBg)
		}
	} else {
		for _, line := range []string{accentStyle.Render("Mesa de commit"), "", dimStyle.Render("Selecione um arquivo para inspecionar."), dimStyle.Render("e abre a árvore completa do repositório."), "", dimStyle.Render("1 Trabalho · 2 Stage · Tab Arquivo")} {
			v.appendLine(line, ColorBg)
		}
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
		bg := ColorBg
		if !v.loading && i+v.offset < len(v.lineBackgrounds) {
			bg = v.lineBackgrounds[i+v.offset]
		}
		lines[i] = surface(line, width, bg)
	}
	return strings.Join(lines, "\n")
}
