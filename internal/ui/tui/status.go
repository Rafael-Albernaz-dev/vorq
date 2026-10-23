package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) panelHeight() int { return max(1, m.height-5) }
func (m *Model) bodyHeight() int  { return max(1, m.panelHeight()-3) }
func (m *Model) paneWidths() (left, center, right int) {
	available := m.width - 2
	left = available * 25 / 100
	right = available * 31 / 100
	center = available - left - right
	return
}

func (m *Model) View() string {
	if m.width < 50 || m.height < 14 {
		return "vorq · Workspace Studio\nAmplie o terminal para pelo menos 50×14.\nq / Ctrl+C para sair."
	}
	mode := "amplo · 3 painéis"
	if !m.isWide {
		mode = "compacto · " + map[string]string{"tree": "navegação", "viewer": "viewer", "commit": "mensagem"}[m.focusPane]
	}
	title := surface(accentStyle.Render(" vorq / MESA DE COMMIT")+fit("", max(1, m.width-ansi.StringWidth(mode)-24))+dimStyle.Render(mode), m.width, ColorRaised)
	branch := "carregando"
	ahead, behind := 0, 0
	if m.studio != nil {
		branch = m.studio.Workspace.Branch
		ahead = m.studio.Changes.Ahead
		behind = m.studio.Changes.Behind
	}
	icons := "texto"
	if m.nerdFonts {
		icons = "Nerd Font"
	}
	contextRight := fmt.Sprintf("%s  ↑%d ↓%d  ícones: %s", displayName(branch), ahead, behind, icons)
	pathWidth := max(1, m.width-ansi.StringWidth(contextRight)-3)
	context := surface(" "+fit(dimStyle.Render(safeText(m.dir)), pathWidth)+" "+cyanStyle.Render(contextRight), m.width, ColorPanel)
	s := m.changeSummary()
	summary := fmt.Sprintf(" %d arquivos alterados  │  +%d no stage  ~%d fora  ?%d novos  !%d conflitos  [-%d] em ambos", s.Total, s.Staged, s.Unstaged, s.Untracked, s.Conflicts, s.Both)
	if m.width < 100 {
		summary = fmt.Sprintf(" %d alterados │ +%d stage ~%d fora ?%d novos !%d U [-%d]", s.Total, s.Staged, s.Unstaged, s.Untracked, s.Conflicts, s.Both)
	}
	top := []string{title, context, surface(summary, m.width, ColorRaised)}
	var body string
	if m.isWide {
		left, center, right := m.paneWidths()
		separator := strings.TrimSuffix(strings.Repeat(lipgloss.NewStyle().Foreground(ColorLine).Background(ColorPanel).Render("│")+"\n", m.panelHeight()), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.treePane(left), separator, m.viewerPane(center), separator, m.commitPane(right))
	} else {
		switch m.focusPane {
		case "viewer":
			body = m.viewerPane(m.width)
		case "commit":
			body = m.commitPane(m.width)
		default:
			body = m.treePane(m.width)
		}
	}
	status := m.notice
	if m.loading {
		status = m.spinner() + " Atualizando workspace…"
	}
	if m.busy != "" {
		status = m.spinner() + " " + m.busy
	}
	if status == "" {
		status = "Pronto · Shift+Tab alterna navegação / viewer / mensagem"
	}
	notice := surface(" "+dimStyle.Render(safeText(status)), m.width, ColorPanel)
	if m.modal == "error" {
		notice = surface(" "+redStyle.Render(safeText(status)), m.width, ColorPanel)
	}
	shortcuts := " a Alterações  e Repositório  1 Trabalho  2 Stage  Tab Arquivo  s Stage  c Mensagem  p Push  w Worktrees  ? Ajuda  q Sair"
	if m.width < 115 {
		shortcuts = " a/e Visões  1/2 Diff  Tab Arquivo  ⇧Tab Foco  s Stage  c Commit  p Push  w Worktrees  q Sair"
	}
	if m.width < 80 {
		shortcuts = " ⇧Tab Foco  a/e Visões  s Stage  c Commit  ?  q Sair"
	}
	if m.focusPane == "commit" {
		shortcuts = " Tab Campos  Enter Editar  Ctrl+G Sugerir  Ctrl+S Revisar  Esc Navegação  Ctrl+C Sair"
		if m.commitEditing {
			shortcuts = " EDITANDO · Tab próximo campo · Esc encerrar edição · Ctrl+S revisar"
		}
		if m.width < 80 {
			shortcuts = " Tab Campos  ^G Sugerir  ^S Revisar  Esc Voltar"
		}
	}
	bar := lipgloss.NewStyle().Foreground(ColorBarText).Background(ColorBar).Render(fit(shortcuts, m.width))
	base := strings.Join(append(append(top, body), notice, bar), "\n")
	if m.modal != "" {
		return overlay(base, m.modalView(), m.width, m.height)
	}
	return base
}
func (m *Model) spinner() string { return []string{"◐", "◓", "◑", "◒"}[m.frame%4] }
func paneRule(width int) string {
	return lipgloss.NewStyle().Foreground(ColorLine).Render(strings.Repeat("─", max(0, width)))
}
func (m *Model) treePane(width int) string {
	tabs := " " + tabLabel("Alterações", m.activeTab == "alteracoes") + " " + tabLabel("Repositório", m.activeTab == "repositorio")
	meta := fmt.Sprintf(" %d alterados · %d arquivos", m.changeSummary().Total, m.inventoryCount)
	if m.focusPane == "tree" {
		meta = " ●" + meta
	} else {
		meta = "  " + meta
	}
	return strings.Join([]string{surface(tabs, width, ColorPanel), surface(dimStyle.Render(meta), width, ColorPanel), surface(paneRule(width), width, ColorPanel), m.treeView(width)}, "\n")
}
func (m *Model) viewerPane(width int) string {
	tabs := " " + tabLabel("Trabalho", m.viewerTab == "trabalho") + " " + tabLabel("Stage", m.viewerTab == "stage") + " " + tabLabel("Arquivo", m.viewerTab == "arquivo")
	breadcrumb := m.viewer.path
	if breadcrumb == "" {
		breadcrumb = "Selecione um arquivo"
	}
	comparison := map[string]string{"trabalho": "índice → working copy", "stage": "HEAD → índice", "arquivo": "conteúdo completo · somente leitura"}[m.viewerTab]
	if node := m.selectedNode(); node != nil && node.Path == m.viewer.path && node.Status.OrigPath != "" {
		breadcrumb = node.Status.OrigPath + " → " + node.Path
	}
	mark := " "
	if m.focusPane == "viewer" {
		mark = "●"
	}
	header := surface(tabs, width, ColorRaised) + "\n" + surface(mark+" "+cyanStyle.Render(displayName(breadcrumb)), width, ColorBg) + "\n" + surface("  "+dimStyle.Render(comparison), width, ColorBg)
	return header + "\n" + m.viewer.view(width, m.bodyHeight())
}

func fieldText(value string, cursor, width, height int, editing bool) []string {
	runes := []rune(value)
	cursor = min(max(0, cursor), len(runes))
	text := safeText(value)
	cursorLine := 0
	if editing {
		before := safeText(string(runes[:cursor]))
		text = before + "▏" + safeText(string(runes[cursor:]))
		cursorLine = strings.Count(ansi.Wrap(before, max(1, width), ""), "\n")
	}
	lines := strings.Split(ansi.Wrap(text, max(1, width), ""), "\n")
	start := 0
	if editing {
		start = max(0, cursorLine-height+1)
	}
	out := make([]string, height)
	for i := range out {
		if start+i < len(lines) {
			out[i] = lines[start+i]
		}
	}
	if height > 0 && start+height < len(lines) {
		out[height-1] = ansi.Truncate(out[height-1]+" …", width, "…")
	}
	return out
}
func (m *Model) fieldBox(value string, cursor, width, rows int, field string) []string {
	active := m.focusPane == "commit" && m.commitField == field
	border := dimStyle
	if active {
		border = accentStyle
	}
	inner := max(1, width-4)
	lines := []string{surface(border.Render("┌"+strings.Repeat("─", width-2)+"┐"), width, ColorPanel)}
	for _, line := range fieldText(value, cursor, inner, rows, active && m.commitEditing) {
		lines = append(lines, surface(border.Render("│")+" "+fit(line, inner)+" "+border.Render("│"), width, ColorPanel))
	}
	return append(lines, surface(border.Render("└"+strings.Repeat("─", width-2)+"┘"), width, ColorPanel))
}
func (m *Model) commitPane(width int) string {
	h := m.panelHeight()
	staged := m.changeSummary().Staged
	label := " MENSAGEM DE COMMIT"
	if m.focusPane == "commit" {
		label = " ● MENSAGEM DE COMMIT"
	}
	header := surface(accentStyle.Render(label), width, ColorRaised)
	summaryLabel := " Resumo"
	if m.commitSummary != "" {
		summaryLabel += fmt.Sprintf(" · %d caracteres", len([]rune(m.commitSummary)))
	}
	confirm := "[Confirmar Commit]  Ctrl+S"
	button := dimStyle
	if staged > 0 && strings.TrimSpace(m.commitSummary) != "" {
		button = accentStyle
	}
	if m.focusPane == "commit" && m.commitField == "actions" {
		confirm = "› " + confirm
	}
	if h < 18 {
		lines := []string{header, surface(dimStyle.Render(summaryLabel), width, ColorPanel)}
		for _, line := range fieldText(m.commitSummary, m.summaryCursor, width-2, 1, m.focusPane == "commit" && m.commitField == "summary" && m.commitEditing) {
			lines = append(lines, surface(" "+line, width, ColorRaised))
		}
		lines = append(lines, surface(dimStyle.Render(" Descrição"), width, ColorPanel))
		rows := max(1, h-8)
		for _, line := range fieldText(m.commitDescription, m.descriptionCursor, width-2, rows, m.focusPane == "commit" && m.commitField == "description" && m.commitEditing) {
			lines = append(lines, surface(" "+line, width, ColorRaised))
		}
		lines = append(lines, surface(greenStyle.Render(fmt.Sprintf(" %d arquivo(s) no stage", staged)), width, ColorPanel), surface(" Ctrl+G · Sugerir Conventional Commit", width, ColorPanel), surface(button.Render(" "+confirm), width, ColorPanel), surface(dimStyle.Render(" Tab campos · Esc sair da edição"), width, ColorPanel))
		return fillPanel(lines, width, h, ColorPanel)
	}
	lines := []string{header, surface(dimStyle.Render(" Local · hooks e assinatura do Git"), width, ColorPanel), surface(paneRule(width), width, ColorPanel), surface(dimStyle.Render(summaryLabel), width, ColorPanel)}
	lines = append(lines, m.fieldBox(m.commitSummary, m.summaryCursor, width, 1, "summary")...)
	lines = append(lines, surface(dimStyle.Render(" Descrição · opcional / multilinha"), width, ColorPanel))
	lines = append(lines, m.fieldBox(m.commitDescription, m.descriptionCursor, width, max(1, min(8, h-17)), "description")...)
	footer := []string{surface(greenStyle.Render(fmt.Sprintf(" %d arquivo(s) preparados", staged)), width, ColorPanel)}
	if m.studio != nil {
		count := 0
		for _, file := range m.studio.Changes.Files {
			if file.IsStaged() && count < 2 {
				footer = append(footer, surface(" · "+displayName(file.Path), width, ColorPanel))
				count++
			}
		}
	}
	if staged == 0 {
		footer = append(footer, surface(dimStyle.Render(" Use s para preparar um arquivo."), width, ColorPanel))
	}
	footer = append(footer, surface("", width, ColorPanel), surface(cyanStyle.Render(" Ctrl+G · Sugerir Conventional Commit"), width, ColorPanel), surface(button.Render(" "+confirm), width, ColorRaised), surface(dimStyle.Render(" Tab: Resumo / Descrição / Confirmar"), width, ColorPanel), surface(dimStyle.Render(" Enter edita · Esc encerra edição"), width, ColorPanel))
	for len(lines)+len(footer) < h {
		lines = append(lines, surface("", width, ColorPanel))
	}
	return fillPanel(append(lines, footer...), width, h, ColorPanel)
}
func fillPanel(lines []string, width, height int, bg lipgloss.Color) string {
	for len(lines) < height {
		lines = append(lines, surface("", width, bg))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) modalView() string {
	width := min(90, m.width-6)
	inner := width - 4
	var title, body, help string
	switch m.modal {
	case "help":
		title = "ATALHOS · MESA DE COMMIT"
		body = "a Alterações · e Repositório (inventário completo)\n1 Trabalho: índice → working copy\n2 Stage: HEAD → índice · Tab: Arquivo completo\nShift+Tab: Árvore → Viewer → Mensagem\nÁrvore: ↑/↓ ou j/k · Enter/Espaço expande pastas\nViewer: ↑/↓ ou j/k · PgUp/PgDown · ←/→ horizontal\ns: stage/unstage do arquivo · c: painel de mensagem\nMensagem: Enter edita · Tab alterna campos\nDescrição: Enter insere uma nova linha\nCtrl+G: sugerir mensagem · Ctrl+S: revisar commit\np: push explícito · w: worktrees · r: atualizar\ni: ícones Nerd Font/texto · Esc: voltar/cancelar\nEm campos de texto, letras são texto, inclusive q.\nq / Ctrl+C: sair (Ctrl+C também durante edição)"
		help = "Esc fecha · PgUp/PgDown rola"
	case "suggest":
		title = "SUGERIR CONVENTIONAL COMMIT"
		body = "Escolha o tipo para os arquivos no stage:\n\n[1] feat   [2] fix    [3] chore    [4] refactor\n[5] docs   [6] test   [7] style    [8] perf\n\nO resumo será preenchido por regras locais.\nA descrição já escrita será preservada."
		help = "1–8 escolher · Esc cancelar"
	case "commit":
		title = "REVISAR COMMIT LOCAL"
		if m.commitPhase == "loading" {
			body = "Inspecionando os arquivos preparados…"
		} else {
			body = fmt.Sprintf("%d arquivo(s) no stage\n\n%s\n\nO commit permanece local. Nenhum push automático.", m.commitCount, safeText(m.reviewMessage))
		}
		help = "Enter confirmar · e editar · Esc cancelar sem unstage"
	case "push":
		title = "PUSH EXPLÍCITO"
		if m.pushTarget == nil {
			body = "Inspecionando destino…"
		} else {
			t := m.pushTarget
			body = fmt.Sprintf("Local: %s -> Remote: %s/%s", displayName(t.Branch), displayName(t.Remote), displayName(t.RemoteBranch))
			if t.HasUpstream && t.Ahead >= 0 {
				body += fmt.Sprintf("\n\n%d commit(s) à frente · %d atrás", t.Ahead, t.Behind)
			}
			if !t.HasUpstream {
				body += "\n\nSem upstream. Publicar e configurar upstream?"
			}
			if t.Behind > 0 {
				body += "\nDivergências serão recusadas. Nenhum force será aplicado."
			}
		}
		help = "Enter confirmar push · Esc cancelar"
	case "worktrees":
		title = "WORKTREES · TROCAR WORKSPACE"
		if len(m.worktrees) == 0 {
			body = "Carregando worktrees…"
		} else {
			var rows []string
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
					s := w.Status.Summary()
					state = fmt.Sprintf(" · %d alterados", s.Total)
				}
				if w.IsBare {
					state = " [bare]"
				}
				rows = append(rows, mark+displayName(w.Branch)+state, "  "+safeText(w.Path))
			}
			body = strings.Join(rows, "\n")
		}
		help = "↑/↓ escolher · Enter abrir · Esc cancelar"
	case "error":
		title = "OPERAÇÃO NÃO CONCLUÍDA"
		body = m.lastError
		help = "Esc voltar · PgUp/PgDown rola · nenhum bypass"
	}
	if m.busy != "" {
		help = m.spinner() + " " + m.busy
		if m.mutating {
			help += " · Ctrl+C encerra"
		} else {
			help += " · Esc cancelar"
		}
	}
	bodyLines := strings.Split(ansi.Wrap(safeText(body), inner, ""), "\n")
	limit := max(1, m.height-10)
	start := min(m.modalScroll, max(0, len(bodyLines)-limit))
	end := min(len(bodyLines), start+limit)
	lines := []string{surface(accentStyle.Render(title), inner, ColorRaised), ""}
	for _, line := range bodyLines[start:end] {
		lines = append(lines, fit(line, inner))
	}
	lines = append(lines, "", cyanStyle.Render(fit(safeText(help), inner)))
	return lipgloss.NewStyle().Foreground(ColorText).Background(ColorPanel).Border(lipgloss.RoundedBorder()).BorderForeground(ColorAccent).Padding(1).Width(width - 2).Render(strings.Join(lines, "\n"))
}
