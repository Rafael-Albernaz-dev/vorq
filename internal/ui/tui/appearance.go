package tui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"regexp"
	"strconv"
	"strings"
)

type studioPalette struct {
	name   string
	colors []string
}

var studioPalettes = []studioPalette{
	{name: "Tokyo Night", colors: []string{"#1a1b26", "#16161e", "#24283b", "#c0caf5", "#9aa5ce", "#414868", "#7aa2f7", "#283457", "#9ece6a", "#f7768e", "#e0af68", "#bb9af7", "#2e4270", "#ffffff", "#7dcfff"}},
	{name: "Dark+", colors: []string{"#1e1e1e", "#252526", "#2d2d30", "#d4d4d4", "#a5a5a5", "#454545", "#66b6ff", "#163d59", "#9bd18b", "#f28b82", "#e4c37c", "#c6a0f6", "#075985", "#ffffff", "#73ccd0"}},
	{name: "Catppuccin", colors: []string{"#1e1e2e", "#181825", "#313244", "#cdd6f4", "#a6adc8", "#45475a", "#b4befe", "#363454", "#a6e3a1", "#f38ba8", "#f9e2af", "#cba6f7", "#494467", "#ffffff", "#89dceb"}},
	{name: "Nord", colors: []string{"#2e3440", "#272d38", "#3b4252", "#eceff4", "#bac5d6", "#4c566a", "#88c0d0", "#3b5061", "#a3be8c", "#f2a0a7", "#ebcb8b", "#c5a4c1", "#3d5b6b", "#ffffff", "#8fbcbb"}},
	{name: "Gruvbox", colors: []string{"#282828", "#242424", "#3c3836", "#ebdbb2", "#bdae93", "#665c54", "#fabd2f", "#50452b", "#b8bb26", "#fb8c7e", "#fabd2f", "#d3869b", "#63521e", "#fff7dc", "#8ec07c"}},
	{name: "Solarized", colors: []string{"#002b36", "#073642", "#10414d", "#c5d5d6", "#93a1a1", "#35606b", "#71c1f0", "#0d4656", "#b4c94f", "#f38b81", "#e5b65c", "#d6a4d5", "#0c536a", "#ffffff", "#65d0c0"}},
	{name: "Papel claro", colors: []string{"#fafafa", "#f0f1f3", "#e7e9ed", "#24292f", "#57606a", "#c6cbd1", "#0550ae", "#dceafc", "#116329", "#a3182a", "#795300", "#6639ba", "#0550ae", "#ffffff", "#095c72"}},
	{name: "Alto contraste", colors: []string{"#0b0b0d", "#111114", "#1e1e24", "#f5f5f7", "#bcbcce", "#68687a", "#80caff", "#253d51", "#a6ed9a", "#ffadad", "#ffe38a", "#dcb9ff", "#183f5c", "#ffffff", "#8fe4f0"}},
}

func (m *Model) themeName() string { return studioPalettes[m.themeIndex%len(studioPalettes)].name }
func colorRGB(hex string) (int, int, int) {
	v, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return int(v >> 16), int(v >> 8 & 255), int(v & 255)
}

// Translate the immutable base design tokens at presentation time. Each model
// owns its palette; changing appearance never mutates shared Lipgloss styles.
var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func (m *Model) applyTheme(frame string) string {
	if m.themeIndex == 0 {
		return frame
	}
	base, target := studioPalettes[0].colors, studioPalettes[m.themeIndex%len(studioPalettes)].colors
	pairs := []string{}
	for i, old := range base {
		r, g, b := colorRGB(old)
		nr, ng, nb := colorRGB(target[i])
		for _, kind := range []int{38, 48} {
			oldSeq, newSeq := fmt.Sprintf("%d;2;%d;%d;%d", kind, r, g, b), fmt.Sprintf("%d;2;%d;%d;%d", kind, nr, ng, nb)
			profile := lipgloss.ColorProfile()
			actualOld, actualNew := profile.Color(old).Sequence(kind == 48), profile.Color(target[i]).Sequence(kind == 48)
			pairs = append(pairs, oldSeq, newSeq)
			if actualOld != "" && actualOld != oldSeq {
				pairs = append(pairs, actualOld, actualNew)
			}
		}
	}
	for _, tint := range []struct {
		old string
		idx int
	}{{"#292f2d", 8}, {"#322531", 9}} {
		r, g, b := colorRGB(tint.old)
		br, bg, bb := colorRGB(target[0])
		fr, fg, fb := colorRGB(target[tint.idx])
		pairs = append(pairs, fmt.Sprintf("48;2;%d;%d;%d", r, g, b), fmt.Sprintf("48;2;%d;%d;%d", (br*89+fr*11)/100, (bg*89+fg*11)/100, (bb*89+fb*11)/100))
	}
	replacer := strings.NewReplacer(pairs...)
	return sgrPattern.ReplaceAllStringFunc(frame, func(sgr string) string {
		if strings.HasPrefix(sgr, "\x1b[39;38;5;") {
			return sgr
		}
		return replacer.Replace(sgr)
	})
}
