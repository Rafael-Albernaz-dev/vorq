package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/charmbracelet/x/ansi"
)

func TestStreamingSyntaxPreservesChromaFormatting(t *testing.T) {
	tokens := []chroma.Token{{Type: chroma.Keyword, Value: "package"}, {Type: chroma.Text, Value: " main\n"}, {Type: chroma.CommentMultiline, Value: "/* ação\ncomment */"}, {Type: chroma.LiteralString, Value: "\"wide 🔷\""}, {Type: chroma.Error, Value: "?"}, {Type: chroma.NameVariableMagic, Value: "value"}, {Type: chroma.TextWhitespace, Value: "\n\n"}}
	for _, palette := range studioPalettes {
		style := highlightStyle(palette.name)
		var expected bytes.Buffer
		if err := formatters.Get("terminal256").Format(&expected, style, chroma.Literator(tokens...)); err != nil {
			t.Fatal(err)
		}
		want := syntaxBackground.ReplaceAllString(expected.String(), "")
		want = strings.ReplaceAll(want, "\x1b[38;5;", "\x1b[39;38;5;")
		actual, err := renderSyntaxTokens(context.Background(), style, chroma.Literator(tokens...), 256)
		if err != nil || actual != want {
			t.Fatalf("%s formatter changed: %q want %q err %v", palette.name, actual, want, err)
		}
	}
}

func TestInactiveWorkspaceReleasesPreviewBuffers(t *testing.T) {
	m, _, _, dir, _ := setupStudio(t)
	m.viewer.offset, m.viewer.column = 2, 7
	m.saveWorkspaceContext()
	saved := m.contexts[dir].viewer
	if saved.content != nil || saved.diff != nil || saved.directory != nil || saved.renderCache != nil || saved.renderContent != nil || saved.renderDiff != nil || len(saved.lines) != 0 || len(saved.lineBackgrounds) != 0 {
		t.Fatal("inactive context retained source/highlight buffers")
	}
	if saved.path != m.viewer.path || saved.offset != 2 || saved.column != 7 {
		t.Fatal("releasing buffers lost navigation")
	}
	if m.viewer.content == nil || len(m.viewer.lines) == 0 {
		t.Fatal("active preview was discarded")
	}
}

func TestGrammarRecoveryAndUnknownLanguagePreserveSource(t *testing.T) {
	color256(t)
	for _, source := range []string{"package main\nfunc greet(name string) {\nreturn name", "// partial hunk\nfunc f() { return \"ação\" }", "package main\nvar x = `unfinished"} {
		spans, available := parseSyntaxSpans(context.Background(), "partial.go", source)
		if !available || len(spans) == 0 {
			t.Fatal("recoverable source bypassed grammar")
		}
		if got := highlightCodeSnippet("partial.go", source, "Dark+"); ansi.Strip(got) != source {
			t.Fatal("recovery altered source")
		}
	}
	if _, available := parseSyntaxSpans(context.Background(), "unknown.extension", "plain"); available {
		t.Fatal("unknown grammar claimed support")
	}
	if got := highlightCodeSnippet("unknown.extension", "literal data", "Dark+"); ansi.Strip(got) != "literal data" {
		t.Fatal("fallback changed source")
	}
}
