package tui

import (
	"fmt"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type IconMode string

const (
	IconModeEmoji IconMode = "emoji"
	IconModeText  IconMode = "text"
)

func ParseIconMode(value string) (IconMode, error) {
	mode := IconMode(value)
	switch mode {
	case IconModeEmoji, IconModeText:
		return mode, nil
	}
	return "", fmt.Errorf("modo de ícones inválido %q: use emoji ou text", value)
}

func (m *Model) currentIconMode() IconMode {
	if m.iconMode == IconModeText {
		return IconModeText
	}
	return IconModeEmoji
}
func (m *Model) SetIconMode(mode IconMode) {
	m.iconMode = mode
}
func (m *Model) cycleIcons() {
	if m.currentIconMode() == IconModeEmoji {
		m.SetIconMode(IconModeText)
	} else {
		m.SetIconMode(IconModeEmoji)
	}
}
func (m *Model) iconModeLabel() string {
	return map[IconMode]string{IconModeEmoji: "Emoji", IconModeText: "Texto"}[m.currentIconMode()]
}

func nodeIcon(node *domain.TreeNode, open bool, mode IconMode) string {
	if node.IsDir {
		if mode == IconModeText {
			return "[DIR]"
		}
		if open {
			return "📂"
		}
		return "📁"
	}
	token := fileIcon(node, open)
	if mode == IconModeText {
		return token
	}
	switch token {
	case "[GO]":
		return "🔷"
	case "[TS]":
		return "🟦"
	case "[JS]":
		return "🟨"
	case "[JSX]", "[TSX]":
		return "⚛️"
	case "[PY]":
		return "🐍"
	case "[HTML]":
		return "🌐"
	case "[CSS]":
		return "🎨"
	case "[MD]":
		return "📝"
	case "[JSON]":
		return "📋"
	case "[ENV]", "[YML]":
		return "⚙️"
	case "[GIT]":
		return "🔀"
	case "[MOD]":
		return "📦"
	case "[IMG]":
		return "🖼️"
	case "[SH]":
		return "📜"
	default:
		return "📄"
	}
}
