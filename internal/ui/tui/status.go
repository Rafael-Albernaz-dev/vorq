package tui

import (
	"fmt"
	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) panelHeight() int {
	if !m.isWide {
		return max(1, m.height-6)
	}
	return max(1, m.height-8)
}
func (m *Model) bodyTop() int {
	if m.isWide {
		return 6
	}
	return 4
}
func (m *Model) bodyHeight() int { return max(1, m.panelHeight()-5) }
func (m *Model) paneWidths() (left, center, right int) {
	available := m.width - 2
	left = available * 25 / 100
	right = available * 31 / 100
	center = available - left - right
	return
}

func (m *Model) View() string { return m.applyTheme(m.renderView()) }
func (m *Model) renderView() string {
	if m.width < 50 || m.height < 14 {
		return "vorq · Workspace Studio\nAmplie o terminal para pelo menos 50×14.\nq / Ctrl+C para sair."
	}
	if m.screen != "" {
		return m.workflowView()
	}
	if m.fullscreen {
		base := m.fullscreenView()
		if m.modal != "" {
			return overlay(base, m.modalView(), m.width, m.height)
		}
		return base
	}
	title := m.repositoryHeader()
	contextRight := m.contextRight()
	pathWidth := max(1, m.width-ansi.StringWidth(contextRight)-3)
	context := surface(" "+fit(dimStyle.Render(safeText(m.dir)), pathWidth)+" "+contextRight, m.width, ColorPanel)
	summary := m.summaryBar()
	if m.studio == nil {
		summary = " … arquivos alterados  │  … no stage  … fora  … novos  … conflitos"
	}
	summaryRow := m.summaryRow(summary)
	top := []string{title, context, summaryRow}
	var body string
	if m.isWide {
		top = []string{title, surface(paneRule(m.width), m.width, ColorRaised), context, surface(paneRule(m.width), m.width, ColorPanel), summaryRow, surface(paneRule(m.width), m.width, ColorBg)}
		left, center, right := m.paneWidths()
		separator := strings.TrimSuffix(strings.Repeat(lipgloss.NewStyle().Foreground(ColorLine).Background(ColorPanel).Render("│")+"\n", m.panelHeight()), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top, m.treePane(left), separator, m.viewerPane(center), separator, m.commitPane(right))
	} else {
		top = append(top, m.compactNavigation())
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
		status = "M modificado · A adicionado · D excluído · ? novo · U conflito │ Stage por arquivo"
	}
	notice := surface(" "+dimStyle.Render(safeText(status)), m.width, ColorPanel)
	if m.modal == "error" || m.stale {
		notice = surface(" "+redStyle.Render(safeText(status)), m.width, ColorPanel)
	}
	shortcuts := " b Branch  H Histórico  P PRs  o GitHub  a Alterações  e Repositório  s Stage  A: Stage tudo · U: Unstage tudo  x Descartar  c Mensagem  p Push  w Worktrees  ? Ajuda  q Sair"
	if m.width < 115 {
		shortcuts = " b Branch  H Histórico  P PRs  a/e Visões  s Stage  A/U Tudo  x Descartar  c Commit  p Push  w Worktrees  ? Ajuda"
	}
	if m.width < 80 {
		shortcuts = " b Branch  H Histórico  P PRs  ⇧Tab Foco  s Stage  c Commit  ?  q"
	}
	if m.focusPane == "commit" && m.activeRightTab() != "commit" {
		shortcuts = " ←/→ Abas  Tab/↑/↓ Ações  Enter Abrir  c Commit  Esc Navegação  q Sair"
	} else if m.focusPane == "commit" {
		shortcuts = " Tab Campos  Enter Editar  Ctrl+G Sugerir  Ctrl+S Revisar  Esc Navegação  Ctrl+C Sair"
		if m.commitEditing {
			shortcuts = " EDITANDO · Tab próximo campo · Esc encerrar edição · Ctrl+S revisar"
		}
		if m.width < 80 {
			shortcuts = " Tab Campos  ^G Sugerir  ^S Revisar  Esc Voltar"
		}
	}
	shortcuts = " [IA] Sugerir " + strings.TrimSpace(shortcuts)
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
	tabs := renderTabs([]string{"Alterações", "Repositório"}, boolIndex(m.activeTab == "repositorio"), width)
	meta := fmt.Sprintf(" %d alterados · %d arquivos carregados", m.changeSummary().Total, m.inventoryCount)
	if m.studio == nil {
		meta = " … alterados · … arquivos"
	}
	legend := " [x] stage · [-] em ambos · s alternar"
	if m.activeTab == "repositorio" {
		legend = " ? novo · ! ignorado · botão direito: copiar path"
	}
	return strings.Join([]string{tabs, surface(dimStyle.Render(meta), width, ColorPanel), m.treeView(width), surface(dimStyle.Render(legend), width, ColorPanel)}, "\n")
}
func boolIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}
func (m *Model) compactNavigation() string {
	labels := []string{"Arquivos", "Conteúdo", "Ações"}
	panes := []string{"tree", "viewer", "commit"}
	out := ""
	for i, label := range labels {
		w := m.width / 3
		if i == 2 {
			w = m.width - 2*w
		}
		style := dimStyle
		bg := ColorPanel
		if panes[i] == m.focusPane {
			style = accentStyle
			bg = ColorSelection
		}
		out += surface(style.Render(" "+label), w, bg)
	}
	return out
}
func (m *Model) viewerPane(width int) string {
	idx := map[string]int{"trabalho": 0, "stage": 1, "arquivo": 2}[m.viewerTab]
	tabs := renderTabs([]string{"Trabalho", "Stage", "Arquivo"}, idx, width)
	breadcrumb := m.viewer.path
	if breadcrumb == "" {
		breadcrumb = "Selecione um arquivo"
	}
	comparison := map[string]string{"trabalho": "índice → trabalho", "stage": "HEAD → índice", "arquivo": "somente leitura"}[m.viewerTab]
	if node := m.selectedNode(); node != nil && node.Path == m.viewer.path && node.Status.OrigPath != "" {
		breadcrumb = node.Status.OrigPath + " → " + node.Path
	}
	context := cyanStyle.Render(displayName(breadcrumb)) + dimStyle.Render(" · "+comparison)
	if m.viewer.path != "" && m.viewer.directory == nil {
		context = redStyle.Bold(true).Render("[Descartar]") + " " + context
	}
	return strings.Join([]string{tabs, surface(" "+context, width, ColorBg), m.viewer.view(width, m.bodyHeight()), surface(dimStyle.Render(m.viewerFooter()), width, ColorPanel)}, "\n")
}

func fieldText(value string, cursor, width, height int, editing bool) []string {
	return fieldLines(value, cursor, width, height, editing, 0)
}
func fieldLines(value string, cursor, width, height int, editing bool, offset int) []string {
	runes := []rune(value)
	cursor = min(max(0, cursor), len(runes))
	text := safeText(value)
	cursorLine := 0
	if editing {
		before := safeText(string(runes[:cursor]))
		text = before + "▏" + safeText(string(runes[cursor:]))
		cursorLine = strings.Count(editorWrap(before+"▏", max(1, width)), "\n")
	}
	lines := strings.Split(editorWrap(text, max(1, width)), "\n")
	start := max(0, offset)
	if editing && (cursorLine < start || cursorLine >= start+height) {
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
	offset := 0
	if field == "description" {
		offset = m.descriptionOffset
	}
	for _, line := range fieldLines(value, cursor, inner, rows, active && m.commitEditing, offset) {
		lines = append(lines, surface(border.Render("│")+" "+fit(line, inner)+" "+border.Render("│"), width, ColorPanel))
	}
	return append(lines, surface(border.Render("└"+strings.Repeat("─", width-2)+"┘"), width, ColorPanel))
}
func (m *Model) commitEditorPane(width int) string {
	h := m.panelHeight()
	staged := m.changeSummary().Staged
	header := m.actionTabs(width)
	ready := staged > 0 && strings.TrimSpace(m.commitSummary) != "" && !m.loading && !m.viewer.loading && !m.stale && m.changeSummary().Conflicts == 0 && m.busy == "" && !m.mutating
	button := dimStyle
	if ready {
		button = accentStyle
	}
	count := fmt.Sprintf(" [x] %d arquivos no stage", staged)
	if staged == 0 {
		count = " [ ] Nenhum arquivo no stage"
	}
	if m.studio == nil {
		count = " [ ] … arquivos no stage"
	}
	if h < 26 {
		lines := []string{header, surface(dimStyle.Render(" Resumo"), width, ColorPanel)}
		for _, line := range fieldText(m.commitSummary, m.summaryCursor, width-2, 1, m.focusPane == "commit" && m.commitField == "summary" && m.commitEditing) {
			lines = append(lines, surface(" "+line, width, ColorRaised))
		}
		if h > 8 {
			lines = append(lines, surface(dimStyle.Render(" Descrição"), width, ColorPanel))
		}
		for _, line := range fieldLines(m.commitDescription, m.descriptionCursor, width-2, max(1, h-8), m.focusPane == "commit" && m.commitField == "description" && m.commitEditing, m.descriptionOffset) {
			lines = append(lines, surface(" "+line, width, ColorRaised))
		}
		lines = append(lines, surface(greenStyle.Render(count), width, ColorPanel), surface(cyanStyle.Bold(true).Render(" [IA] Sugerir com IA · modelo"), width, ColorPanel), surface(button.Render(" [c] Revisar commit local"), width, ColorPanel), surface(dimStyle.Render(" Local · Tab campos · Esc voltar"), width, ColorPanel))
		return fillPanel(lines, width, h, ColorPanel)
	}
	lines := []string{header, surface(paneRule(width), width, ColorPanel), surface(cyanStyle.Bold(true).Render(" [IA] Sugerir com IA · modelo"), width, ColorPanel), surface("", width, ColorPanel), surface(dimStyle.Render(" Resumo"), width, ColorPanel)}
	lines = append(lines, m.fieldBox(m.commitSummary, m.summaryCursor, width, 1, "summary")...)
	lines = append(lines, surface(dimStyle.Render(" Descrição · opcional"), width, ColorPanel))
	lines = append(lines, m.fieldBox(m.commitDescription, m.descriptionCursor, width, 4, "description")...)
	lines = append(lines, surface("", width, ColorPanel), surface(greenStyle.Render(count), width, ColorPanel))
	mark := "[ ]"
	if m.suggested {
		mark = "[x]"
	}
	source := "Escolha IA ou regras locais"
	if m.suggested {
		source = m.suggestionSource
		if source == "" {
			source = "Sugestão aplicada"
		}
	}
	lines = append(lines, surface(dimStyle.Render(" "+mark+" "+safeText(source)), width, ColorPanel), surface(dimStyle.Render(" [ ] Revisar diff preparado"), width, ColorPanel), surface("", width, ColorPanel))
	lines = append(lines, surface(button.Render("┌"+strings.Repeat("─", width-2)+"┐"), width, ColorPanel), surface(button.Render("│"+fit(" [c] Revisar commit local", width-2)+"│"), width, ColorPanel), surface(button.Render("└"+strings.Repeat("─", width-2)+"┘"), width, ColorPanel))
	lines = append(lines, surface("", width, ColorPanel), surface(dimStyle.Render(" Commit local; push é separado."), width, ColorPanel), surface(dimStyle.Render(" Cancelar preserva stage e rascunho."), width, ColorPanel))
	return fillPanel(lines, width, h, ColorPanel)
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
	if m.modal == "diff-review" {
		width = min(140, m.width-4) // long technical text reads better wider
	}
	inner := width - 4
	var title, body, help, buttons string
	if m.modal == "workflow" {
		return m.workflowFormView()
	}
	switch m.modal {
	case "result":
		title = "COMMIT LOCAL CONCLUÍDO"
		if m.resultKind == "pull" {
			title = "ATUALIZAÇÃO CONCLUÍDA"
		}
		if m.resultKind == "push" {
			title = "PUSH CONCLUÍDO"
		}
		body = m.resultBody
		help = "Resultado confirmado pelo Git nativo · Enter ou Esc voltar"
		buttons = "[Voltar]"
	case "appearance":
		title = "APARÊNCIA"
		body = "Paletas da Mesa de Commit · tema salvo para a próxima sessão\n\n"
		for i, p := range studioPalettes {
			mark := "  "
			if i == m.themeIndex {
				mark = "› "
			}
			body += fmt.Sprintf("%s[%d] %s\n", mark, i+1, p.name)
		}
		body += "\n[i] Ícones Emoji ↔ Texto\nModo: " + m.iconModeLabel()
		body += " · " + nodeIcon(&domain.TreeNode{Name: "file.go"}, false, m.currentIconMode()) + " Go"

		help = "1–8 escolher · i ícones · Esc voltar"
		buttons = "[Voltar]"
	case "discard":
		title = "DESCARTAR ARQUIVO"
		body = "Verificando arquivo…"
		buttons = "[Cancelar]"
		if m.discardPlan != nil {
			body = "Caminho exato: " + displayName(m.discardPlan.Path) + "\n\n" + m.discardPlan.Impact
			buttons = "[Descartar] [Cancelar]"
			if m.discardPlan.Delete && m.discardPlan.Staged {
				body += "\n\nOpções: Enter exclui permanentemente; u apenas retira do stage e preserva o arquivo."
				buttons = "[Descartar] [Preservar] [Cancelar]"
			}
		}
		help = "Enter descartar · u preservar novo staged · Esc cancelar"
	case "unstage":
		title = "RETIRAR ARQUIVO DO STAGE"
		body = "O arquivo tem alterações preparadas e fora do stage.\n\nRetirar do stage remove toda a preparação deste arquivo. O conteúdo da cópia de trabalho será preservado.\n\n"
		if n := m.selectedNode(); n != nil {
			body += safeText(n.Path)
		}
		help = "Enter retirar do stage · Esc voltar"
		buttons = "[Confirmar]  [Voltar]"
	case "help":
		title = "ATALHOS · MESA DE COMMIT"
		body = "a Alterações · e Repositório (inventário completo)\n1 Trabalho: índice → working copy\n2 Stage: HEAD → índice · Tab: Arquivo completo\nShift+Tab: Árvore → Viewer → Mensagem\nÁrvore: ↑/↓ ou j/k · Enter/Espaço expande pastas\nViewer: ↑/↓ ou j/k · PgUp/PgDown · ←/→ horizontal\ns: stage/unstage do arquivo · c: painel de mensagem\nA: Stage tudo · U: Unstage tudo\nx/d: descartar arquivo (sempre confirmar)\nMensagem: Enter edita · Tab alterna campos\nDescrição: Enter insere uma nova linha\nCtrl+G: sugerir mensagem · Ctrl+S: revisar commit\np: push explícito · w: worktrees\nf ou [Tela cheia]: arquivo/diff em tela cheia (z quebra linhas, Esc sai)\nr ou [↻ Atualizar]: reler o workspace (sem atualização automática)\nPainel direito: abas Commit/Git/PRs/Worktrees clicáveis\nSetas da branch: ↑ revisar push · ↓ revisar atualização ff-only\nb: nova branch · H: histórico · P: PRs · o: abrir GitHub\ni: ícones Emoji/texto · t: aparência · Esc: voltar/cancelar\nEm campos de texto, letras são texto, inclusive q.\nq / Ctrl+C: sair (Ctrl+C também durante edição)"
		help = "Esc fecha · PgUp/PgDown ou roda rola"
		buttons = "[Voltar]"
	case "suggest":
		title = "SUGERIR CONVENTIONAL COMMIT"
		body = "Sugerir com IA (inglês):\n" + providerMenu + "\n\nRegras locais — escolha o tipo:\n[1] feat   [2] fix    [3] chore    [4] refactor\n[5] docs   [6] test   [7] style    [8] perf\n\nIA: prévia antes de substituir o rascunho.\nRegras: preservam a descrição existente."
		help = "c/g/a IA · 1–8 regras locais · Esc cancelar"
		if m.aiTask != "" && m.aiTask != "suggest" {
			body = "Escolha o provedor:\n" + providerMenu + "\n\nDepois escolha o modelo. Inglês por padrão."
			help = "c/g/a provedor · Esc voltar"
		}
		buttons = "[Voltar]"
	case "ai-send":
		title = "SUGERIR COM IA · " + m.aiProvider
		body = "Modelo: " + safeText(m.aiModelLabel()) + "\nEnviar somente o diff staged ao provedor escolhido.\n" + fmt.Sprintf("Inglês · até %d KiB · timeout de %d min.", app.MaxAIDiffBytes/1024, int(app.AISuggestTimeout.Minutes())) + "\n\nO conteúdo sai desta máquina e usa a cota da assinatura.\nA resposta será revisada antes de substituir resumo e descrição.\nNenhum commit ou push será executado."
		if m.aiTask == "review" || m.aiTask == "plan" {
			body = "Modelo: " + safeText(m.aiModelLabel()) + "\nAnalisar versões de trabalho das mudanças rastreadas e novos arquivos elegíveis, incluindo fora do stage.\nIgnorados, binários, links e arquivos grandes serão listados como excluídos.\n\n" + fmt.Sprintf("Até %d MiB por análise e 1 MiB por arquivo; sem truncamento silencioso.", app.AnalysisLimit/(1024*1024)) + "\nO conteúdo sai desta máquina e usa a cota da assinatura.\nNenhum stage ou commit até aprovar o plano."
		}
		if m.changeSummary().Staged == 0 && m.aiTask != "review" && m.aiTask != "plan" {
			body = "Nenhum arquivo no stage.\nVolte e use s em um arquivo ou A para preparar as mudanças.\n\nModelo escolhido: " + safeText(m.aiModelLabel()) + "\nNada será enviado sem arquivos no stage."
		}
		if m.busy != "" {
			body = "Analisando somente o staged…\nEsc cancela e preserva seu rascunho."
			if m.aiTask == "review" || m.aiTask == "plan" {
				body = "Analisando mudanças elegíveis…\nEsc cancela a análise; arquivos e stage preservados."
			}
		}
		help = "Enter enviar · Esc cancelar"
		buttons = "[Confirmar]  [Voltar]"
	case "ai-models":
		title = "ESCOLHER MODELO · " + m.aiProvider
		body = m.aiModelsBody()
		if m.busy != "" {
			body = "Consultando modelos disponíveis…"
		}
		help = "↑/↓ ou clique escolher · Enter revisar envio · Esc voltar"
		buttons = "[Revisar] [Voltar]"
	case "ai-review":
		title = "REVISAR SUGESTÃO · " + m.aiProvider
		if m.aiProposal != nil {
			body = safeText(m.aiProposal.Proposal.Message())
		}
		body += "\n\nUsar preenche o editor. Commit abre a revisão local antes de confirmar.\nAmbos substituem resumo e descrição do rascunho."
		help = "Enter usar · v revisar commit · o opções · Esc voltar"
		buttons = "[Usar] [Commit] [Mais] [Voltar]"
	case "ai-error":
		title = "SUGESTÃO NÃO GERADA · " + m.aiProvider
		body = safeText(m.aiError) + "\n\nSeu rascunho foi preservado.\nAbra Opções para repetir, escolher outro provedor ou usar regras locais."
		help = "o opções · r repetir · c/g/a provedor · l regras · Esc voltar"
		buttons = "[Opções] [Voltar]"
	case "ai-options":
		title = "OPÇÕES DA SUGESTÃO"
		body = "[r] Tentar novamente\n[c] Codex\n[g] Gemini (Antigravity)\n[a] Claude Code\n[l] Regras locais\n\nO rascunho atual permanece preservado."
		help = "Clique ou r/c/g/a/l escolher · Esc fechar"
		buttons = "[Voltar]"
	case "diff-review":
		title = "REVISÃO DO DIFF · IA"
		help = "o organizar commits · PgUp/PgDown/roda rolar · Esc voltar"
		buttons = "[Organizar] [Voltar]"
	case "plan":
		title = "PLANO DE COMMITS · REVISAR"
		body = m.planBody()
		help = "↑/↓ grupo · e editar · d diff · Enter executar · Esc voltar"
		buttons = "[Executar] [Editar] [Diff] [Voltar]"
		if inner < 43 {
			buttons = "[OK] [Editar] [Diff] [Voltar]"
		}
	case "plan-diff":
		title = "DIFF DO GRUPO · CONTEÚDO REVISADO"
		body = m.planDiffBody()
		help = "PgUp/PgDown rolar · Esc voltar"
		buttons = "[Voltar]"
	case "plan-edit":
		title = "EDITAR PLANO · " + m.planEditKind
		instructions := map[string]string{"message": "Mensagem: primeira linha título, demais descrição. Ctrl+U limpa.", "move": "Informe ID e grupo destino. Exemplo: F0001 2", "exclude": "Informe ID a excluir. Arquivo/stage serão preservados.", "order": "Informe o número da posição destino do grupo."}
		body = instructions[m.planEditKind] + "\n\n" + safeText(m.planEditValue) + "▏"
		help = "Ctrl+S salvar · Esc cancelar"
		buttons = "[Salvar] [Cancelar]"
	case "plan-execute":
		title = "APROVAR E EXECUTAR PLANO"
		body = m.planBody() + "\n\nConfirmar cria a sequência restante de commits locais.\nSe uma etapa falhar, anteriores podem existir; o progresso será salvo.\nSem push. Nenhum stash, descarte ou reescrita do histórico."
		help = "Enter confirmar sequência · Esc cancelar"
		buttons = "[Confirmar] [Voltar]"
	case "commit":
		title = "REVISAR COMMIT LOCAL"
		if m.commitPhase == "loading" {
			body = "Inspecionando os arquivos preparados…"
		} else {
			body = fmt.Sprintf("%d arquivo(s) no stage\n\n%s\n\nO commit permanece local. Nenhum push automático.", m.commitCount, safeText(m.reviewMessage))
			body += "\n\nArquivos preparados:\n" + strings.Join(m.reviewPaths, "\n")
		}
		help = "Enter confirmar · e editar · Esc cancelar sem unstage"
		buttons = "[Confirmar]  [Editar]  [Voltar]"
	case "branches":
		title = "TROCAR BRANCH · BRANCHES LOCAIS"
		body = "Carregando branches…"
		if m.busy == "" {
			rows, _ := m.branchModalRows()
			body = strings.Join(rows, "\n")
			if len(rows) == 0 {
				body = "Nenhuma branch local disponível."
			}
		}
		help = "Clique ou ↑/↓ escolher · Enter revisar · Esc voltar"
		buttons = "[Revisar] [Voltar]"
	case "switch-branch":
		title = "REVISAR TROCA DE BRANCH"
		body = "Verificando destino e estado local…"
		if p := m.branchSwitchPlan; p != nil {
			body = fmt.Sprintf("Origem: %s\nDestino: %s\n\nWorkspace: %s\n\nTroca local com workspace limpo. Stage e arquivos ignorados são protegidos. Nenhum stash, descarte ou acesso à rede.", p.From, p.Target, p.WorkspacePath)
		}
		help = "Enter confirmar troca · Esc cancelar"
		buttons = "[Confirmar] [Voltar]"
	case "pull":
		title = "ATUALIZAR BRANCH · FAST-FORWARD"
		body = "Inspecionando origem e estado local…"
		if m.pullTarget != nil {
			t := m.pullTarget
			body = fmt.Sprintf("Origem: %s/%s → Local: %s\n\n↑%d à frente · ↓%d atrás (referências locais)\n\nBuscar atualizações e aplicar somente fast-forward?\n\nSem merge, rebase, stash ou descarte automático. Divergências e colisões com arquivos ignorados são recusadas. A busca pode atualizar referências remotas mesmo se a aplicação for bloqueada.", t.Remote, t.RemoteBranch, t.Branch, t.Ahead, t.Behind)
		}
		help = "Enter confirmar busca e atualização · Esc cancelar"
		buttons = "[Confirmar] [Voltar]"
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
		buttons = "[Confirmar]  [Voltar]"
	case "worktrees":
		title = "WORKTREES · TROCAR WORKSPACE"
		if len(m.worktrees) == 0 {
			body = "Carregando worktrees…"
		} else {
			rows, _ := m.worktreeModalRows()
			body = strings.Join(rows, "\n")
		}
		help = "↑/↓ escolher · Enter abrir · Esc cancelar"
		buttons = "[Abrir]  [Voltar]"
	case "error":
		title = "OPERAÇÃO NÃO CONCLUÍDA"
		if strings.HasPrefix(m.lastError, "Commit criado;") {
			title = "COMMIT CRIADO · AVISO DE LEITURA"
		}
		body = m.lastError
		help = "Esc voltar · PgUp/PgDown rola · nenhum bypass"
		buttons = "[Voltar]"
		if m.terminalRetryKind != "" {
			body += "\n\nO Git/SSH/GPG precisou pedir uma credencial, o que o Studio não permite na própria tela. Nada foi publicado ou commitado nesta tentativa.\n\n[Terminal] (ou t): o Studio é suspenso, a mesma operação revisada roda no terminal normal (com revalidação) e o Studio volta ao terminar."
			help = "t tentar no terminal · Esc voltar"
			buttons = terminalRetryButton + "  [Voltar]"
		}
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
	styled := m.modal == "diff-review" && m.diffReview != nil
	if styled {
		bodyLines = m.diffReviewLines(inner) // sanitised and wrapped per piece
	}
	limit := m.modalBodyHeight()
	start := min(m.modalScroll, max(0, len(bodyLines)-limit))
	end := min(len(bodyLines), start+limit)
	lines := []string{surface(accentStyle.Render(title), inner, ColorRaised), ""}
	for _, line := range bodyLines[start:end] {
		if styled {
			lines = append(lines, surface(line, inner, ColorPanel))
		} else {
			lines = append(lines, fit(line, inner))
		}
	}
	lines = append(lines, "", dimStyle.Render(fit(safeText(help), inner)))
	lines = append(lines, outlinedButtons(buttons, inner)...)
	return lipgloss.NewStyle().Foreground(ColorText).Background(ColorPanel).Border(lipgloss.NormalBorder()).BorderForeground(ColorAccent).Padding(1).Width(width - 2).Render(strings.Join(lines, "\n"))
}

// editorWrap keeps words together and still bounds long tokens to terminal
// cells. Unlike Wrap's combined path, this handles the Unicode caret safely.
func editorWrap(text string, width int) string {
	return ansi.Hardwrap(ansi.Wordwrap(text, max(1, width), ""), max(1, width), true)
}
func outlinedButtons(buttons string, width int) []string {
	top, middle, bottom := "", "", ""
	for _, label := range strings.Fields(buttons) {
		w := ansi.StringWidth(label)
		if middle != "" {
			top += " "
			middle += " "
			bottom += " "
		}
		top += "┌" + strings.Repeat("─", w) + "┐"
		middle += "│" + label + "│"
		bottom += "└" + strings.Repeat("─", w) + "┘"
	}
	return []string{cyanStyle.Render(fit(top, width)), cyanStyle.Render(fit(middle, width)), cyanStyle.Render(fit(bottom, width))}
}

func (m *Model) modalBodyHeight() int { return max(1, m.height-11) }

// viewerFooter is shared by the pane and the full-screen viewer.
func (m *Model) viewerFooter() string {
	comparison := map[string]string{"trabalho": "índice → trabalho", "stage": "HEAD → índice", "arquivo": "somente leitura"}[m.viewerTab]
	if m.viewer.directory != nil {
		comparison = "resumo da pasta"
	}
	footer := fmt.Sprintf(" +%d −%d linhas · %s · %d/%d", m.viewer.added, m.viewer.deleted, comparison, min(len(m.viewer.lines), m.viewer.offset+1), len(m.viewer.lines))
	if m.viewerTab == "arquivo" && m.viewer.content != nil {
		c := m.viewer.content
		if c.Notice != "" {
			footer = " " + safeText(c.Notice)
		} else {
			count := 0
			if c.Text != "" {
				count = strings.Count(strings.TrimSuffix(c.Text, "\n"), "\n") + 1
			}
			footer = fmt.Sprintf(" %d linhas · somente leitura", count)
		}
	}
	if m.viewer.directory != nil {
		footer = fmt.Sprintf(" %d caminhos · pasta", m.viewer.directory.FileCount())
	}
	if m.viewer.combined {
		footer = " Diff de conflito · marcadores dos pais preservados"
	}
	if m.viewer.binary {
		footer = " Binário · sem preview de texto"
	}
	return footer
}
