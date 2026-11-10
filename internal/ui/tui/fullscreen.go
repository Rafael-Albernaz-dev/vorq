package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// VORQ-013 full-screen viewer. It shows only the current file/diff using the
// whole terminal. Entering or leaving never re-reads anything; the viewer
// scroll position is shared with the normal layout. Only stage (s) and
// discard (x, always confirmed) act on the file; everything else requires
// leaving with Esc, f or q.

const (
	// Toggle shown at the right end of the summary row (normal layout) and of
	// the title row (full screen), as requested: "Tela cheia [ ]" / "[x]".
	fullscreenOff = "Tela cheia [ ]"
	fullscreenOn  = "Tela cheia [x]"
	exitButton    = fullscreenOn
	wrapButton    = "[z Quebrar linhas"
	stageButton   = "[s Stage]"
	discardButton = "[x Descartar]"
)

var viewerTabLabels = []string{"Trabalho", "Stage", "Arquivo"}
var viewerTabIDs = []string{"trabalho", "stage", "arquivo"}

// canFullscreen: only a loaded file view (never a directory summary).
func (m *Model) canFullscreen() bool {
	node := m.selectedNode()
	return m.viewer.path != "" && m.viewer.directory == nil && node != nil && !node.IsDir && node.Path == m.viewer.path
}

// summaryRowY is the screen row of the change summary (and of the toggle).
func (m *Model) summaryRowY() int {
	if m.isWide {
		return 4
	}
	return 2
}

// summaryRow renders the change summary with the full-screen toggle aligned
// to the right edge of the same row.
func (m *Model) summaryRow(summary string) string {
	toggle := dimStyle.Render(fullscreenOff)
	if m.canFullscreen() {
		toggle = accentStyle.Bold(true).Render(fullscreenOff)
	}
	space := max(1, m.width-ansi.StringWidth(fullscreenOff)-1)
	return surface(fit(summary, space)+toggle, m.width, ColorBg)
}

func (m *Model) enterFullscreen() {
	if !m.canFullscreen() {
		m.notice = "Selecione um arquivo para abrir em tela cheia."
		return
	}
	m.fullscreen, m.fullscreenPath = true, m.viewer.path
	m.focusPane, m.commitEditing = "viewer", false
}

func (m *Model) exitFullscreen() {
	m.fullscreen, m.fullscreenPath = false, ""
	m.viewer.scroll(0, m.bodyHeight()) // re-clamp for the smaller pane
}

// fullscreenBodyHeight: title row, three tab rows, info row and shortcut bar.
func (m *Model) fullscreenBodyHeight() int { return max(1, m.height-6) }

func (m *Model) fullscreenButtons() string {
	wrap := wrapButton + "]"
	if m.viewerWrap {
		wrap = wrapButton + " ✓]"
	}
	// The exit toggle always stays visible; other buttons are dropped from the
	// left as the terminal narrows (their keys keep working).
	buttons := []string{stageButton, discardButton, wrap, exitButton}
	for len(buttons) > 1 && ansi.StringWidth(strings.Join(buttons, "  "))+12 > m.width {
		buttons = buttons[1:]
	}
	return strings.Join(buttons, "  ")
}

func (m *Model) fullscreenTitle() string {
	buttons := m.fullscreenButtons()
	comparison := map[string]string{"trabalho": "índice → trabalho", "stage": "HEAD → índice", "arquivo": "somente leitura"}[m.viewerTab]
	path := m.viewer.path
	if node := m.selectedNode(); node != nil && node.Path == path && node.Status.OrigPath != "" {
		path = node.Status.OrigPath + " → " + path
	}
	left := " " + cyanStyle.Bold(true).Render(displayName(path)) + dimStyle.Render(" · "+comparison)
	space := max(1, m.width-ansi.StringWidth(buttons)-1)
	return surface(fit(left, space)+accentStyle.Bold(true).Render(buttons), m.width, ColorRaised)
}

func (m *Model) fullscreenView() string {
	idx := map[string]int{"trabalho": 0, "stage": 1, "arquivo": 2}[m.viewerTab]
	body := m.viewer.view(m.width, m.fullscreenBodyHeight())
	if m.viewerWrap {
		body = m.viewer.wrappedView(m.width, m.fullscreenBodyHeight())
	}
	status := m.notice
	if m.busy != "" {
		status = m.spinner() + " " + m.busy
	}
	info := dimStyle.Render(m.viewerFooter())
	if status != "" {
		info += dimStyle.Render("  ·  " + safeText(status))
	}
	shortcuts := " Esc/f/q sair · ↑/↓ PgUp/PgDn rolar · ←/→ horizontal · z quebrar linhas · 1/2/Tab abas · s stage · x descartar"
	bar := surface(fit(shortcuts, m.width), m.width, ColorBar)
	return strings.Join([]string{m.fullscreenTitle(), renderTabs(viewerTabLabels, idx, m.width), body, surface(info, m.width, ColorPanel), bar}, "\n")
}

// wrappedView renders source lines from offset, soft-wrapping each one to the
// width; offsets stay in source lines so toggling wrap keeps the position.
func (v *viewerState) wrappedView(width, height int) string {
	lines := make([]string, 0, height)
	for i := v.offset; i < len(v.lines) && len(lines) < height; i++ {
		bg := ColorBg
		if !v.loading && i < len(v.lineBackgrounds) {
			bg = v.lineBackgrounds[i]
		}
		for _, segment := range strings.Split(ansi.Hardwrap(v.lines[i], width, true), "\n") {
			if len(lines) == height {
				break
			}
			lines = append(lines, surface(segment, width, bg))
		}
	}
	for len(lines) < height {
		lines = append(lines, surface("", width, ColorBg))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) fullscreenScroll(delta int) {
	height := m.fullscreenBodyHeight()
	if m.viewerWrap {
		height = 1 // offsets are source lines; allow reaching the last one
	}
	m.viewer.scroll(delta, height)
}

func (m *Model) fullscreenKey(key tea.KeyMsg) tea.Cmd {
	h := m.fullscreenBodyHeight()
	switch key.String() {
	case "esc", "f", "q":
		m.exitFullscreen()
	case "z":
		m.viewerWrap = !m.viewerWrap
		if m.viewerWrap {
			m.viewer.column = 0
		}
	case "up", "k":
		m.fullscreenScroll(-1)
	case "down", "j":
		m.fullscreenScroll(1)
	case "pgup":
		m.fullscreenScroll(-h)
	case "pgdown", " ":
		m.fullscreenScroll(h)
	case "home", "g":
		m.viewer.offset = 0
	case "end", "G":
		m.viewer.offset = 0
		m.fullscreenScroll(len(m.viewer.lines))
	case "left", "h":
		if !m.viewerWrap {
			m.viewer.column = max(0, m.viewer.column-4)
		}
	case "right", "l":
		if !m.viewerWrap {
			m.viewer.column += 4
		}
	case "1":
		m.setViewerTab("trabalho")
	case "2":
		m.setViewerTab("stage")
	case "tab":
		if m.viewerTab == "arquivo" {
			tab := m.lastDiffTab
			if tab == "" {
				tab = "trabalho"
			}
			m.setViewerTab(tab)
		} else {
			m.lastDiffTab = m.viewerTab
			m.setViewerTab("arquivo")
		}
	case "s":
		return m.stage()
	case "x", "d":
		return m.inspectDiscard()
	}
	return nil
}

func (m *Model) fullscreenMouse(msg tea.MouseMsg) tea.Cmd {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.fullscreenScroll(-3)
		return nil
	case tea.MouseButtonWheelDown:
		m.fullscreenScroll(3)
		return nil
	case tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight:
		if !m.viewerWrap {
			delta := 3
			if msg.Button == tea.MouseButtonWheelLeft {
				delta = -3
			}
			m.viewer.column = max(0, m.viewer.column+delta)
		}
		return nil
	case tea.MouseButtonLeft:
	default:
		return nil
	}
	switch {
	case msg.Y == 0:
		title := m.fullscreenTitle()
		for label, key := range map[string]string{exitButton: "esc", wrapButton: "z", stageButton: "s", discardButton: "x"} {
			if mouseLabelHit(title, label, msg.X) {
				if key == "esc" {
					return m.fullscreenKey(tea.KeyMsg{Type: tea.KeyEsc})
				}
				return m.fullscreenKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			}
		}
	case msg.Y >= 1 && msg.Y <= 3:
		if index := mouseTabAt(viewerTabLabels, m.width, msg.X); index >= 0 {
			m.setViewerTab(viewerTabIDs[index])
		}
	}
	return nil
}
