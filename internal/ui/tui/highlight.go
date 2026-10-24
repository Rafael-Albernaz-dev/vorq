package tui

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Chroma error tokens can have explicit backgrounds (e.g. GitHub/Monokai).
// The viewer exclusively owns backgrounds, including invalid/incomplete code.
var syntaxBackground = regexp.MustCompile(`\x1b\[48;5;[0-9]+m`)

func chromaTheme(theme string) string {
	switch theme {
	case "Tokyo Night":
		return "tokyonight-night" // Chroma's registered name.
	case "Dark+":
		return "vs"
	case "Catppuccin":
		return "catppuccin-mocha"
	case "Nord":
		return "nord"
	case "Gruvbox":
		return "gruvbox"
	case "Solarized":
		return "solarized-dark"
	case "Papel claro":
		return "github"
	case "Alto contraste":
		return "monokai"
	default:
		return "tokyonight-night"
	}
}

// Tokenise the complete source once so multiline comments/strings retain their
// state. terminal256 resets each physical line so viewport cuts, gutters and
// diff backgrounds remain independent. Never accept repository escapes.
func highlightCodeLines(filename, source, theme string) []string {
	return highlightCodeLinesContext(context.Background(), filename, source, theme)
}

func highlightCodeLinesContext(ctx context.Context, filename, source, theme string) []string {
	source = safeText(source)
	if source == "" {
		return nil
	}
	plain := strings.Split(strings.TrimSuffix(source, "\n"), "\n")
	if lipgloss.ColorProfile() == termenv.Ascii {
		return plain
	}
	if ctx.Err() != nil {
		return plain
	}
	lexer := lexers.Match(filename)
	if name := filepath.Base(filename); name == "go.mod" || name == "go.sum" || name == "go.work" {
		lexer = goManifestLexer
	}
	if lexer == nil {
		lexer = lexers.Analyse(source)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, source)
	if err != nil {
		return plain
	}
	iterator = withSymbolRoles(iterator, structuralSpans(ctx, filename, source))
	style := highlightStyle(theme)
	formatter := formatters.Get("terminal256")
	var out bytes.Buffer
	checked := func() chroma.Token {
		if ctx.Err() != nil {
			return chroma.EOF
		}
		return iterator()
	}
	if err := formatter.Format(&out, style, checked); err != nil || ctx.Err() != nil {
		return plain
	}
	// terminal256 closes every token (and physical newline) with an SGR reset.
	result := strings.Split(syntaxBackground.ReplaceAllString(out.String(), ""), "\n")
	if len(result) < len(plain) {
		return plain
	}
	result = result[:len(plain)]
	for i := range result {
		// 39 resets only the foreground. Mark syntax colours so the chrome's
		// palette translation cannot remap a coincidentally equal ANSI colour.
		result[i] = strings.ReplaceAll(result[i], "\x1b[38;5;", "\x1b[39;38;5;")
	}
	return result
}

func highlightCodeSnippet(filename, source, theme string) string {
	return strings.Join(highlightCodeLines(filename, source, theme), "\n")
}

// Highlight each hunk's old/new sides as separate snippets. Prefixes remain
// owned by the diff renderer, and metadata never enters a language lexer.
func highlightDiffLines(filename, text, theme string) map[int]string {
	return highlightDiffLinesContext(context.Background(), filename, text, theme)
}

func highlightDiffLinesContext(ctx context.Context, filename, text, theme string) map[int]string {
	lines := strings.Split(strings.TrimSuffix(safeText(text), "\n"), "\n")
	result := map[int]string{}
	oldSource, newSource := []string{}, []string{}
	oldRows, newRows := []int{}, []int{}
	flush := func() {
		for _, side := range []struct {
			source []string
			rows   []int
		}{{oldSource, oldRows}, {newSource, newRows}} {
			if len(side.rows) == 0 {
				continue
			}
			colored := highlightCodeLinesContext(ctx, filename, strings.Join(side.source, "\n")+"\n", theme)
			for i, row := range side.rows {
				if i < len(colored) {
					result[row] = colored[i]
				}
			}
		}
		oldSource, newSource, oldRows, newRows = nil, nil, nil, nil
	}
	inHunk := false
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			flush()
			inHunk = hunkPattern.MatchString(line)
			continue
		}
		if !inHunk || len(line) == 0 {
			continue
		}
		switch line[0] {
		case '-':
			oldSource = append(oldSource, line[1:])
			oldRows = append(oldRows, i)
		case '+':
			newSource = append(newSource, line[1:])
			newRows = append(newRows, i)
		case ' ':
			oldSource = append(oldSource, line[1:])
			oldRows = append(oldRows, i)
			newSource = append(newSource, line[1:])
			newRows = append(newRows, i)
		}
	}
	flush()
	return result
}
