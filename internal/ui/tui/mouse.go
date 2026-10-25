package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// mousePane uses the same widths and height budget as the renderer. Separators,
// context bars and the footer are deliberately outside every action area.
func (m *Model) mousePane(x, y int) (pane string, localX, localY, width int) {
	top := m.bodyTop()
	if y < top || y >= top+m.panelHeight() {
		return "", 0, 0, 0
	}
	if !m.isWide {
		return m.focusPane, x, y - top, m.width
	}
	left, center, right := m.paneWidths()
	switch {
	case x < left:
		return "tree", x, y - top, left
	case x > left && x < left+1+center:
		return "viewer", x - left - 1, y - top, center
	case x > left+1+center:
		return "commit", x - left - center - 2, y - top, right
	}
	return "", 0, 0, 0
}

// mouseLabelHit measures terminal cells, rather than UTF-8 bytes. Labels are
// read from the rendered line so their visible positions remain authoritative.
func mouseLabelHit(line, label string, x int) bool {
	line = ansi.Strip(line)
	index := strings.Index(line, label)
	if index < 0 {
		return false
	}
	start := ansi.StringWidth(line[:index])
	return x >= start && x < start+ansi.StringWidth(label)
}

func (m *Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.width < 50 || m.height < 14 || msg.X < 0 || msg.Y < 0 || msg.X >= m.width || msg.Y >= m.height {
		return m, nil
	}
	// Release and motion are never action events, including after a press that
	// opened a dialog or started a mutation.
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if m.modal == "workflow" {
		return m, m.workflowFormMouse(msg)
	}
	if m.modal != "" {
		return m, m.modalMouse(msg)
	}
	if m.screen != "" {
		return m, m.workflowMouse(msg)
	}
	if msg.Button == tea.MouseButtonLeft {
		if hit, cmd := m.contextArrowClick(msg.X, msg.Y); hit {
			return m, cmd
		}
	}
	if msg.Y == 0 && msg.Button == tea.MouseButtonLeft {
		return m, m.openRepository()
	}
	wheel := msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight
	if msg.Button != tea.MouseButtonLeft && !wheel {
		return m, nil
	}
	if msg.Y == m.height-1 && !wheel {
		return m, m.mouseFooter(msg.X)
	}
	if !m.isWide && msg.Y == 3 && !wheel {
		m.commitEditing = false
		index := 0
		if msg.X >= m.width/3 {
			index = 1
		}
		if msg.X >= 2*(m.width/3) {
			index = 2
		}
		m.focusPane = []string{"tree", "viewer", "commit"}[index]
		return m, nil
	}
	pane, x, y, width := m.mousePane(msg.X, msg.Y)
	if pane == "" {
		return m, nil
	}
	if wheel {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelLeft {
			delta = -3
		}
		switch pane {
		case "tree":
			if y >= 4 && y < 4+m.bodyHeight() && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
				m.treeOffset = min(max(0, m.treeOffset+delta), max(0, len(m.rows)-m.bodyHeight()))
			}
		case "viewer":
			if y >= 4 && y < m.panelHeight()-1 {
				if msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
					m.viewer.column = max(0, m.viewer.column+delta)
				} else {
					m.viewer.scroll(delta, m.bodyHeight())
				}
			}
		case "commit":
			if m.activeRightTab() != "commit" {
				return m, nil
			}
			_, start, rows, inner := m.mouseDescriptionRect(width)
			if y >= start && y < start+rows {
				count := len(strings.Split(editorWrap(safeText(m.commitDescription), inner), "\n"))
				offset := mouseFieldStart(m.commitDescription, m.descriptionCursor, inner, rows, m.focusPane == "commit" && m.commitField == "description" && m.commitEditing, m.descriptionOffset)
				m.descriptionOffset = min(max(0, offset+delta), max(0, count-rows))
				if m.focusPane == "commit" && m.commitField == "description" {
					m.commitEditing = false
				}
			}
		}
		return m, nil
	}
	m.focusPane = pane
	if pane != "commit" {
		m.commitEditing = false
	}
	switch pane {
	case "tree":
		if y >= 0 && y < 3 {
			index := mouseTabAt([]string{"Alterações", "Repositório"}, width, x)
			if index >= 0 {
				m.switchTree([]string{"alteracoes", "repositorio"}[index])
				return m, m.loadSelected()
			}
		}
		index := m.treeOffset + y - 4
		if y < 4 || y >= 4+m.bodyHeight() || index < 0 || index >= len(m.rows) || m.rows[index].node == nil {
			return m, nil
		}
		m.selected = index
		node := m.selectedNode()
		if node.IsDir {
			m.expanded[m.expandedKey()] = !m.expanded[m.expandedKey()]
			m.rebuildTree()
			return m, m.loadSelected()
		}
		if x >= 3 && x <= 5 {
			// The normal key route applies busy/loading/stale guards as well as
			// any confirmation required for partial staging.
			return m, m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		}
		return m, m.loadSelected()
	case "viewer":
		if y == 3 && m.viewer.path != "" && m.viewer.directory == nil {
			line := strings.Split(m.viewerPane(width), "\n")[3]
			if mouseLabelHit(line, "[Descartar]", x) {
				return m, m.inspectDiscard()
			}
		}
		if y >= 0 && y < 3 {
			index := mouseTabAt([]string{"Trabalho", "Stage", "Arquivo"}, width, x)
			if index >= 0 {
				m.setViewerTab([]string{"trabalho", "stage", "arquivo"}[index])
			}
		}
	case "commit":
		if y == 0 {
			m.selectRightTab(mouseTabAt(rightTabs, width, x))
			return m, nil
		}
		if m.activeRightTab() != "commit" {
			rows := m.actionRows(width)
			if y >= 0 && y < len(rows) && rows[y].action >= 0 {
				m.rightActionIndex = rows[y].action
				return m, m.runPanelAction(rows[y].action)
			}
			return m, nil
		}
		if m.mutating {
			return m, nil
		}
		compact := m.panelHeight() < 26
		sx, sy, sw := 2, 6, max(1, width-4)
		if compact {
			sx, sy, sw = 1, 2, max(1, width-2)
		}
		if y == sy && x >= sx && x < sx+sw {
			editing := m.commitField == "summary" && m.commitEditing
			start := mouseFieldStart(m.commitSummary, m.summaryCursor, sw, 1, editing, 0)
			m.summaryCursor = fieldDisplayCursorAt(m.commitSummary, m.summaryCursor, sw, 0, x-sx, start, editing)
			m.commitField, m.commitEditing = "summary", true
			return m, nil
		}
		dx, dy, rows, inner := m.mouseDescriptionRect(width)
		if y >= dy && y < dy+rows && x >= dx && x < dx+inner {
			editing := m.commitField == "description" && m.commitEditing
			start := mouseFieldStart(m.commitDescription, m.descriptionCursor, inner, rows, editing, m.descriptionOffset)
			m.descriptionCursor = fieldDisplayCursorAt(m.commitDescription, m.descriptionCursor, inner, y-dy, x-dx, start, editing)
			m.commitField, m.commitEditing = "description", true
			m.descriptionOffset = start
			return m, nil
		}
		lines := strings.Split(m.commitPane(width), "\n")
		if y < len(lines) && (mouseLabelHit(lines[y], "Sugerir", x) || mouseLabelHit(lines[y], "Conventional", x)) || !compact && y == 2 || compact && y == m.panelHeight()-3 {
			m.commitEditing = false
			return m, m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlG})
		}
		if compact && y == m.panelHeight()-2 || !compact && y >= 20 && y <= 22 {
			m.commitField, m.commitEditing = "actions", false
			return m, m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlS})
		}
	}
	return m, nil
}

func (m *Model) mouseDescriptionRect(width int) (x, y, rows, inner int) {
	if m.panelHeight() <= 8 {
		return 1, 3, 1, max(1, width-2)
	}
	if m.panelHeight() < 26 {
		return 1, 4, max(1, m.panelHeight()-8), max(1, width-2)
	}
	return 2, 10, 4, max(1, width-4)
}

func mouseTabAt(labels []string, width, x int) int {
	start := 0
	for i, cells := range tabWidths(labels, width) {
		if x >= start && x < start+cells {
			return i
		}
		start += cells
	}
	return -1
}

// The renderer inserts a one-cell caret before wrapping. Include that caret in
// the hit map, then translate back to the underlying draft's rune index.
func fieldDisplayCursorAt(value string, cursor, width, row, column, offset int, editing bool) int {
	if !editing {
		return fieldCursorAt(value, width, row, column, offset)
	}
	runes := []rune(value)
	cursor = min(max(0, cursor), len(runes))
	display := string(runes[:cursor]) + "▏" + string(runes[cursor:])
	index := fieldCursorAt(display, width, row, column, offset)
	if index > cursor {
		index--
	}
	return index
}

func mouseFieldStart(value string, cursor, width, rows int, editing bool, offset int) int {
	start := max(0, offset)
	if editing {
		runes := []rune(value)
		cursor = min(max(0, cursor), len(runes))
		line := strings.Count(editorWrap(safeText(string(runes[:cursor]))+"▏", max(1, width)), "\n")
		if line < start || line >= start+rows {
			start = max(0, line-rows+1)
		}
	}
	return start
}

// fieldCursorAt translates the visible soft-wrapped row and terminal column to
// the editor's rune index. Searching each successive wrapped line preserves
// explicit newlines and whitespace preserved by the renderer without quadratic
// wrapping of every possible prefix.
func fieldCursorAt(value string, width, row, column, offset int) int {
	value = safeText(value)
	lines := strings.Split(editorWrap(value, max(1, width)), "\n")
	target := min(max(0, row+offset), len(lines)-1)
	byteStart := 0
	for i := 0; i <= target; i++ {
		line := lines[i]
		index := strings.Index(value[byteStart:], line)
		if index < 0 {
			index = 0
		}
		byteStart += index
		if i == target {
			prefix := ""
			for _, r := range line {
				if ansi.StringWidth(prefix+string(r)) > max(0, column) {
					break
				}
				prefix += string(r)
			}
			return utf8.RuneCountInString(value[:byteStart]) + utf8.RuneCountInString(prefix)
		}
		byteStart += len(line)
		if byteStart < len(value) && value[byteStart] == '\n' {
			byteStart++
		}
	}
	return utf8.RuneCountInString(value)
}

func (m *Model) modalMouse(msg tea.MouseMsg) tea.Cmd {
	if m.modal == "branches" {
		return m.branchSelectorMouse(msg)
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -3
		}
		if m.modal == "worktrees" {
			m.worktreeIndex = min(max(0, m.worktreeIndex+delta/3), max(0, len(m.worktrees)-1))
		} else {
			m.modalScroll = max(0, m.modalScroll+delta)
		}
		return nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	box := strings.Split(m.modalView(), "\n")
	width := 0
	for _, line := range box {
		width = max(width, ansi.StringWidth(line))
	}
	x, y := msg.X-max(0, (m.width-width)/2), msg.Y-max(0, (m.height-len(box))/2)
	if x < 0 || x >= width || y < 0 || y >= len(box) {
		return nil
	}
	line := box[y]
	// Only the renderer's dedicated three-row outlined footer owns actions.
	// Message text and paths remain data even when they contain button labels.
	for label, key := range map[string]string{"[Descartar]": "enter", "[Preservar]": "u", "[Confirmar]": "enter", "[Abrir]": "enter", "[Editar]": "e", "[Voltar]": "esc", "[Fechar]": "esc", "[Cancelar]": "esc"} {
		if mouseOutlinedButtonHit(box, label, x, y) {
			if key == "enter" {
				return m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			}
			if key == "esc" {
				return m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
			}
			return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		}
	}
	if m.busy != "" {
		return nil
	}
	if m.modal == "appearance" && mouseLabelHit(line, "[i]", x) {
		return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	}
	if m.modal == "suggest" || m.modal == "appearance" {
		for i := 1; i <= 8; i++ {
			label := "[" + string(rune('0'+i)) + "]"
			if mouseLabelHit(line, label, x) {
				return m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{rune('0' + i)}})
			}
		}
	}
	if m.modal == "worktrees" {
		raw, owners := m.worktreeModalRows()
		var wrappedOwners []int
		inner := min(90, m.width-6) - 4
		for i, row := range raw {
			for range strings.Split(ansi.Wrap(safeText(row), inner, ""), "\n") {
				wrappedOwners = append(wrappedOwners, owners[i])
			}
		}
		// Border, padding, title and its following blank occupy four rows;
		// the fixed footer, padding and borders occupy seven more rows.
		visible := len(box) - 11
		if y >= 4 && y < 4+visible {
			start := min(m.modalScroll, max(0, len(wrappedOwners)-visible))
			row := start + y - 4
			if row >= 0 && row < len(wrappedOwners) {
				m.worktreeIndex = wrappedOwners[row]
			}
		}
	}
	return nil
}

// Footer shortcuts are also mouse controls; they go through the same contextual
// keyboard route, so availability, confirmation and editing precedence agree.
func (m *Model) mouseFooter(x int) tea.Cmd {
	lines := strings.Split(m.renderView(), "\n")
	if len(lines) == 0 {
		return nil
	}
	line := lines[len(lines)-1]
	controls := []struct {
		label string
		key   tea.KeyMsg
	}{
		{"b Branch", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}},
		{"H Histórico", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}}},
		{"P PRs", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}}},
		{"o GitHub", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}}},
		{"a Alterações", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}},
		{"e Repositório", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}},
		{"1 Trabalho", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}},
		{"2 Stage", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}},
		{"Tab Arquivo", tea.KeyMsg{Type: tea.KeyTab}},
		{"A: Stage tudo", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}}},
		{"U: Unstage tudo", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}}},
		{"x Descartar", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}},
		{"s Stage", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}},
		{"c Mensagem", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}},
		{"c Commit", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}},
		{"p Push", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}},
		{"w Worktrees", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}}},
		{"? Ajuda", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}},
		{"q Sair", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
		{"⇧Tab Foco", tea.KeyMsg{Type: tea.KeyShiftTab}},
		{"Tab Campos", tea.KeyMsg{Type: tea.KeyTab}},
		{"Ctrl+G Sugerir", tea.KeyMsg{Type: tea.KeyCtrlG}},
		{"Ctrl+S Revisar", tea.KeyMsg{Type: tea.KeyCtrlS}},
		{"Esc Navegação", tea.KeyMsg{Type: tea.KeyEsc}},
		{"Esc Voltar", tea.KeyMsg{Type: tea.KeyEsc}},
		{"Ctrl+C Sair", tea.KeyMsg{Type: tea.KeyCtrlC}},
	}
	for _, control := range controls {
		if mouseLabelHit(line, control.label, x) {
			return m.handleKey(control.key)
		}
	}
	return nil
}

// mouseOutlinedButtonHit shares the rendered footer's label positions and
// includes each button's own border, without accepting tokens from the body.
func mouseOutlinedButtonHit(box []string, label string, x, y int) bool {
	if len(box) < 5 || y < len(box)-5 || y > len(box)-3 {
		return false
	}
	line := ansi.Strip(box[len(box)-4])
	index := strings.Index(line, label)
	if index < 0 {
		return false
	}
	start := ansi.StringWidth(line[:index]) - 1
	return x >= start && x < start+ansi.StringWidth(label)+2
}

// worktreeModalRows retains the real owner of each display row. Matching path
// strings would let a wrapped prefix accidentally select another worktree.
func (m *Model) worktreeModalRows() (rows []string, owners []int) {
	visible := max(1, (m.height-12)/2)
	start := max(0, m.worktreeIndex-visible+1)
	for i := start; i < min(len(m.worktrees), start+visible); i++ {
		w := m.worktrees[i]
		mark := "  "
		if i == m.worktreeIndex {
			mark = "› "
		}
		state := ""
		if w.Status != nil {
			state = fmt.Sprintf(" · %d alterados", w.Status.Summary().Total)
		}
		if w.IsBare {
			state = " [bare]"
		}
		rows = append(rows, mark+displayName(w.Branch)+state, "  "+safeText(w.Path))
		owners = append(owners, i, i)
	}
	return rows, owners
}
