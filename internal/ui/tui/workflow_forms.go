package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func csvValues(s string) []string {
	var result []string
	for _, s := range strings.Split(s, ",") {
		if s = strings.TrimSpace(s); s != "" {
			result = append(result, s)
		}
	}
	return result
}
func field(label, value string, multi, toggle bool) workflowField {
	return workflowField{Label: label, Value: value, Cursor: len([]rune(value)), Multiline: multi, Toggle: toggle}
}
func (m *Model) openBranchForm() {
	if !m.canAct() {
		return
	}
	m.formKind = "branch"
	m.formReview = false
	m.fieldIndex = 0
	m.modalScroll = 0
	m.modal = "workflow"
	m.fields = []workflowField{field("Nome", "", false, false), field("Base (vazio = HEAD)", "", false, false), field("Criar e entrar", "sim", false, true)}
}
func (m *Model) openFilterForm() {
	m.modal = "workflow"
	m.formKind = "filter"
	m.formReview = false
	m.fieldIndex = 0
	m.modalScroll = 0
	if m.screen == "history" {
		m.fields = []workflowField{field("Buscar mensagem", m.historyOptions.Search, false, false), field("Branch/tag (vazio = todas)", m.historyOptions.Ref, false, false)}
	} else {
		m.fields = []workflowField{field("Busca GitHub", m.prOptions.Search, false, false), field("Estado (all/open/closed/merged)", m.prOptions.State, false, false)}
	}
}
func (m *Model) preparePR() tea.Cmd {
	ctx, cancel, id := m.workflowContext()
	m.modal = "workflow"
	m.fields = nil
	m.busy = "Preparando PR a partir dos commits…"
	dir, core := m.dir, m.core
	return func() tea.Msg {
		defer cancel()
		d, e := core.PreparePullRequest(ctx, dir, "")
		return workflowMsg{id: id, kind: "prepare", dir: dir, draft: d, err: e}
	}
}
func (m *Model) setupPRForm() {
	d := m.prDraft
	m.formKind = "pr"
	m.modal = "workflow"
	m.fieldIndex = 0
	m.formReview = false
	m.modalScroll = 0
	draft := "não"
	if d.Draft {
		draft = "sim"
	}
	m.fields = []workflowField{field("Título", d.Title, false, false), field("Descrição", d.Body, true, false), field("Base", d.Base, false, false), field("Labels a adicionar (vírgulas)", "", false, false), field("Reviewers (vírgulas)", "", false, false)}
	if d.Number == 0 {
		m.fields = append(m.fields, field("Draft", draft, false, true))
	} else {
		m.fields = append(m.fields, field("Labels a remover (vírgulas)", "", false, false))
	}
}
func (m *Model) workflowFormKey(k tea.KeyMsg) tea.Cmd {
	s := k.String()
	if s == "esc" {
		if m.mutating {
			return nil
		}
		m.workflowID++
		if m.workflowCancel != nil {
			m.workflowCancel()
		}
		m.busy = ""
		m.modal = ""
		return nil
	}
	if m.busy != "" || len(m.fields) == 0 {
		return nil
	}
	if m.formReview {
		switch s {
		case "enter":
			return m.submitWorkflowForm()
		case "e":
			m.formReview = false
		case "pgdown", "down":
			m.modalScroll += 3
		case "pgup", "up":
			m.modalScroll = max(0, m.modalScroll-3)
		}
		return nil
	}
	if s == "ctrl+s" {
		m.formReview = true
		m.modalScroll = 0
		return nil
	}
	if s == "tab" {
		m.fieldIndex = (m.fieldIndex + 1) % len(m.fields)
		return nil
	}
	if s == "shift+tab" {
		m.fieldIndex = (m.fieldIndex + len(m.fields) - 1) % len(m.fields)
		return nil
	}
	f := &m.fields[m.fieldIndex]
	if f.Toggle {
		if s == " " || s == "enter" {
			if f.Value == "sim" {
				f.Value = "não"
			} else {
				f.Value = "sim"
			}
		}
		return nil
	}
	r := []rune(f.Value)
	f.Cursor = min(max(0, f.Cursor), len(r))
	switch s {
	case "left":
		f.Cursor = max(0, f.Cursor-1)
	case "right":
		f.Cursor = min(len(r), f.Cursor+1)
	case "home", "ctrl+a":
		for f.Cursor > 0 && r[f.Cursor-1] != '\n' {
			f.Cursor--
		}
	case "end", "ctrl+e":
		for f.Cursor < len(r) && r[f.Cursor] != '\n' {
			f.Cursor++
		}
	case "up", "down":
		start, end := f.Cursor, f.Cursor
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		for end < len(r) && r[end] != '\n' {
			end++
		}
		col := f.Cursor - start
		if s == "up" && start > 0 {
			prev := start - 1
			for prev > 0 && r[prev-1] != '\n' {
				prev--
			}
			f.Cursor = prev + min(col, start-1-prev)
		}
		if s == "down" && end < len(r) {
			next := end + 1
			for next < len(r) && r[next] != '\n' {
				next++
			}
			f.Cursor = end + 1 + min(col, next-end-1)
		}
	case "backspace", "ctrl+h":
		if f.Cursor > 0 {
			r = append(r[:f.Cursor-1], r[f.Cursor:]...)
			f.Cursor--
			f.Value = string(r)
		}
	case "delete":
		if f.Cursor < len(r) {
			f.Value = string(append(r[:f.Cursor], r[f.Cursor+1:]...))
		}
	case "ctrl+u":
		f.Value = ""
		f.Cursor = 0
	case "enter":
		if f.Multiline {
			f.Value = string(r[:f.Cursor]) + "\n" + string(r[f.Cursor:])
			f.Cursor++
		} else {
			m.fieldIndex = (m.fieldIndex + 1) % len(m.fields)
		}
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			text := string(k.Runes)
			if k.Type == tea.KeySpace {
				text = " "
			}
			text = strings.Map(func(r rune) rune {
				if r == '\n' && f.Multiline {
					return r
				}
				if r == '\n' || r == '\r' {
					return ' '
				}
				if unicode.IsControl(r) {
					return -1
				}
				return r
			}, text)
			if len(r)+len([]rune(text)) > 65536 {
				return nil
			}
			if !f.Multiline {
				text = strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\r", " ")
			}
			f.Value = string(r[:f.Cursor]) + text + string(r[f.Cursor:])
			f.Cursor += len([]rune(text))
		}
	}
	return nil
}
func (m *Model) submitWorkflowForm() tea.Cmd {
	if m.mutating || m.busy != "" {
		return nil
	}
	if m.formKind == "filter" {
		m.modal = ""
		if m.screen == "history" {
			m.historyOptions.Search = m.fields[0].Value
			m.historyOptions.Ref = m.fields[1].Value
			m.historyOptions.Skip = 0
			return m.loadHistory()
		}
		m.prOptions.Search = m.fields[0].Value
		m.prOptions.State = m.fields[1].Value
		return m.loadPRs()
	}
	m.dialogID++
	id, core, dir, ctx := m.dialogID, m.core, m.dir, m.ctx
	m.mutating = true
	m.busy = "Executando operação confirmada…"
	if m.formKind == "branch" {
		o := domain.BranchOptions{Name: m.fields[0].Value, Base: m.fields[1].Value, Stay: m.fields[2].Value != "sim"}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			e := core.CreateBranch(ctx, dir, o)
			return actionMsg{id: id, kind: "branch", notice: "Branch criada: " + o.Name, err: e}
		}
	}
	d := m.prDraft
	d.Title = m.fields[0].Value
	d.Body = m.fields[1].Value
	d.Base = m.fields[2].Value
	d.Labels = csvValues(m.fields[3].Value)
	d.Reviewers = csvValues(m.fields[4].Value)
	if d.Number == 0 {
		d.Draft = m.fields[5].Value == "sim"
	} else {
		d.RemoveLabels = csvValues(m.fields[5].Value)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		result, e := core.SavePullRequest(ctx, dir, d)
		return actionMsg{id: id, kind: "pr", notice: "PR atualizado: " + result, err: e}
	}
}
func (m *Model) workflowFormView() string {
	width := min(90, m.width-6)
	inner := width - 4
	title := "NOVA BRANCH"
	if m.formKind == "pr" {
		base := m.prDraft.Base
		if len(m.fields) > 2 {
			base = m.fields[2].Value
		}
		prefix := "NOVO PR"
		if m.prDraft.Number > 0 {
			prefix = fmt.Sprintf("EDITAR PR #%d", m.prDraft.Number)
		}
		title = fmt.Sprintf("%s · %s · %s → %s", prefix, m.prDraft.Repository, m.prDraft.Head, base)
	}
	if m.formKind == "filter" {
		title = "FILTRAR"
	}
	lines := []string{accentStyle.Render(fit(safeText(title), inner)), ""}
	help := "Tab campos · Enter próximo/nova linha · Espaço alterna · Ctrl+S revisar · Esc cancelar"
	buttons := "[Revisar] [Cancelar]"
	if m.formReview {
		body := ""
		if m.formKind == "pr" {
			body = fmt.Sprintf("Repositório: https://github.com/%s\nBranch de origem: %s\n\n", m.prDraft.Repository, m.prDraft.Head)
		}
		for _, f := range m.fields {
			body += f.Label + ": " + f.Value + "\n\n"
		}
		body += "Confira o destino e os valores antes de confirmar."
		if m.formKind == "branch" {
			body += "\nStage e arquivos serão preservados; nenhuma publicação."
		}
		wrapped := strings.Split(ansi.Hardwrap(safeText(body), inner, true), "\n")
		h := max(1, m.height-13)
		start := min(m.modalScroll, max(0, len(wrapped)-h))
		for _, l := range wrapped[start:min(len(wrapped), start+h)] {
			lines = append(lines, fit(l, inner))
		}
		help = "Enter confirmar · e editar · PgUp/PgDn revisar · Esc cancelar"
		buttons = "[Confirmar] [Editar] [Cancelar]"
	} else {
		available := max(1, m.height-11)
		compact := len(m.fields)+3 > available
		for i, f := range m.fields {
			if compact && i != m.fieldIndex {
				continue
			}
			mark := "  "
			if i == m.fieldIndex {
				mark = "› "
			}
			value := strings.ReplaceAll(safeText(f.Value), "\n", " ↵ ")
			lines = append(lines, fit(fmt.Sprintf("%s[%d/%d] %s: %s", mark, i+1, len(m.fields), f.Label, value), inner))
		}
		if len(m.fields) > 0 && available > 1 {
			f := m.fields[m.fieldIndex]
			used := len(m.fields)
			if compact {
				used = 1
			}
			height := max(1, min(4, available-used))
			for _, l := range fieldLines(f.Value, f.Cursor, inner, height, true, 0) {
				lines = append(lines, fit(l, inner))
			}
		}
	}
	if m.busy != "" {
		help = m.spinner() + " " + m.busy
		buttons = "[Cancelar]"
	}
	lines = append(lines, "", dimStyle.Render(fit(help, inner)))
	lines = append(lines, outlinedButtons(buttons, inner)...)
	return lipgloss.NewStyle().Foreground(ColorText).Background(ColorPanel).Border(lipgloss.NormalBorder()).BorderForeground(ColorAccent).Padding(1).Width(width - 2).Render(strings.Join(lines, "\n"))
}
func (m *Model) workflowFormMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress {
		return nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.formReview {
			m.modalScroll += 3
		}
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		m.modalScroll = max(0, m.modalScroll-3)
		return nil
	}
	if msg.Button != tea.MouseButtonLeft {
		return nil
	}
	box := strings.Split(m.workflowFormView(), "\n")
	width := 0
	for _, l := range box {
		width = max(width, ansi.StringWidth(l))
	}
	x, y := msg.X-max(0, (m.width-width)/2), msg.Y-max(0, (m.height-len(box))/2)
	if y < 0 || y >= len(box) || x < 0 || x >= width {
		return nil
	}
	for label, key := range map[string]tea.KeyType{"[Confirmar]": tea.KeyEnter, "[Cancelar]": tea.KeyEsc, "[Revisar]": tea.KeyCtrlS} {
		if mouseOutlinedButtonHit(box, label, x, y) {
			return m.workflowFormKey(tea.KeyMsg{Type: key})
		}
	}
	if mouseOutlinedButtonHit(box, "[Editar]", x, y) {
		return m.workflowFormKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	}
	if !m.formReview && m.busy == "" {
		for i, f := range m.fields {
			prefix := "[" + strconv.Itoa(i+1) + "/" + strconv.Itoa(len(m.fields)) + "] " + f.Label + ":"
			if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ansi.Strip(box[y])), "│")), prefix) || strings.Contains(ansi.Strip(box[y]), "› "+prefix) {
				m.fieldIndex = i
				return nil
			}
		}
	}
	return nil
}
