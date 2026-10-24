package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) editKey(key tea.KeyMsg) {
	value, cursor := &m.commitSummary, &m.summaryCursor
	if m.commitField == "description" {
		value, cursor = &m.commitDescription, &m.descriptionCursor
	}
	runes := []rune(*value)
	*cursor = min(max(0, *cursor), len(runes))
	pos := *cursor
	changed := false
	start := pos
	for start > 0 && runes[start-1] != '\n' {
		start--
	}
	end := pos
	for end < len(runes) && runes[end] != '\n' {
		end++
	}
	switch key.String() {
	case "enter":
		if m.commitField == "summary" {
			m.commitField = "description"
			return
		}
		runes = append(append(append([]rune(nil), runes[:pos]...), '\n'), runes[pos:]...)
		*cursor++
		changed = true
	case "left":
		*cursor = max(0, pos-1)
	case "right":
		*cursor = min(len(runes), pos+1)
	case "home", "ctrl+a":
		*cursor = start
	case "end", "ctrl+e":
		*cursor = end
	case "up":
		if start > 0 {
			previous := start - 1
			for previous > 0 && runes[previous-1] != '\n' {
				previous--
			}
			*cursor = previous + min(pos-start, start-1-previous)
		}
	case "down":
		if end < len(runes) {
			nextEnd := end + 1
			for nextEnd < len(runes) && runes[nextEnd] != '\n' {
				nextEnd++
			}
			*cursor = end + 1 + min(pos-start, nextEnd-end-1)
		}
	case "ctrl+u":
		runes = nil
		*cursor = 0
		changed = true
	case "backspace", "ctrl+h":
		if pos > 0 {
			runes = append(runes[:pos-1], runes[pos:]...)
			*cursor--
			changed = true
		}
	case "delete":
		if pos < len(runes) {
			runes = append(runes[:pos], runes[pos+1:]...)
			changed = true
		}
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			inserted := key.Runes
			if key.Type == tea.KeySpace {
				inserted = []rune{' '}
			}
			inserted = []rune(strings.Map(func(r rune) rune {
				if r == '\n' && m.commitField == "description" {
					return r
				}
				if r == '\n' {
					return ' '
				}
				if unicode.IsControl(r) {
					return -1
				}
				return r
			}, string(inserted)))
			limit := 65536
			if m.commitField == "summary" {
				limit = 4096
			}
			if len(runes)+len(inserted) > limit {
				return
			}
			tail := append([]rune(nil), runes[pos:]...)
			runes = append(runes[:pos], inserted...)
			runes = append(runes, tail...)
			*cursor += len(inserted)
			changed = true
		}
	}
	if changed {
		*value = string(runes)
		m.draftRevision++
	}
}
