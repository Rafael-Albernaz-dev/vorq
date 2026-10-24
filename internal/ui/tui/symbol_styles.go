package tui

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"sync"
)

var symbolStyles sync.Map

// Immutable, per-theme roles. Chroma's lexical palette remains responsible for
// keywords, strings, comments, operators and numbers.
func highlightStyle(theme string) *chroma.Style {
	key := chromaTheme(theme)
	if cached, ok := symbolStyles.Load(key); ok {
		return cached.(*chroma.Style)
	}
	colors := []string{"#e0af68", "#7dcfff", "#ff9e64", "#9ece6a", "#bb9af7"}
	switch theme {
	case "Dark+":
		colors = []string{"#dcdcaa", "#9cdcfe", "#ffb86c", "#4ec9b0", "#9cdcfe"}
	case "Catppuccin":
		colors = []string{"#f9e2af", "#89dceb", "#fab387", "#a6e3a1", "#cba6f7"}
	case "Nord":
		colors = []string{"#ebcb8b", "#88c0d0", "#d08770", "#a3be8c", "#b48ead"}
	case "Gruvbox":
		colors = []string{"#fabd2f", "#83a598", "#fe8019", "#b8bb26", "#d3869b"}
	case "Solarized":
		colors = []string{"#b58900", "#268bd2", "#cb4b16", "#859900", "#6c71c4"}
	case "Papel claro":
		colors = []string{"#795e26", "#005cc5", "#af5c00", "#267f99", "#6f42c1"}
	case "Alto contraste":
		colors = []string{"#ffe38a", "#8fe4f0", "#ffb86c", "#a6ed9a", "#dcb9ff"}
	}
	builder := styles.Get(key).Builder()
	for role, color := range map[chroma.TokenType]string{
		chroma.NameFunction: colors[0], chroma.NameVariable: colors[1], chroma.NameVariableMagic: colors[2],
		chroma.NameClass: colors[3], chroma.NameProperty: colors[4], chroma.NameAttribute: colors[1],
		chroma.NameTag: colors[4], chroma.NameConstant: colors[4], chroma.NameVariableGlobal: colors[1],
	} {
		builder.Add(role, color)
	}
	style, err := builder.Build()
	if err != nil {
		return styles.Get(key)
	}
	actual, _ := symbolStyles.LoadOrStore(key, style)
	return actual.(*chroma.Style)
}
