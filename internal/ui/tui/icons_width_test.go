package tui

import (
	"testing"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

// Every emoji icon must be one code point, without a variation selector, so the
// width measured by the layout matches what terminals draw (VORQ-011, U9).
func TestEmojiIconsHaveStableWidth(t *testing.T) {
	names := []string{"a.go", "go.mod", ".gitignore", "LICENSE", "a.tsx", "a.jsx", "a.ts", "a.js", "a.py", "a.html", "a.css", "a.md", "a.json", "a.yaml", "a.sh", "a.png", "a.toml", ".env", "a.bin"}
	nodes := []*domain.TreeNode{{IsDir: true}}
	for _, name := range names {
		nodes = append(nodes, &domain.TreeNode{Name: name})
	}
	for _, node := range nodes {
		for _, open := range []bool{false, true} {
			icon := nodeIcon(node, open, IconModeEmoji)
			if utf8.RuneCountInString(icon) != 1 {
				t.Errorf("%q icon %q must be a single code point (no VS16/ZWJ)", node.Name, icon)
			}
			if w := ansi.StringWidth(icon); w != 2 {
				t.Errorf("%q icon %q has width %d, want 2", node.Name, icon, w)
			}
		}
	}
}
