package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) bodyHeight() int { return max(1, m.height-5) }

func (m *Model) View() string {
	if m.width < 50 || m.height < 14 {
		return "vorq · Workspace Studio\nAmplie o terminal para pelo menos 50×14.\nq / Ctrl+C para sair."
	}
	left := max(16, m.width*30/100)
	right := m.width - left - 3
	branch := "carregando…"
	counts := ""
	if m.studio != nil {
		cs := m.studio.Changes
		branch = displayName(m.studio.Workspace.Branch)
		counts = fmt.Sprintf("↑%d ↓%d   +%d ~%d ?%d", cs.Ahead, cs.Behind, cs.StagedCount(), cs.UnstagedCount(), cs.UntrackedCount())
	}
	brand := accentStyle.Render(" vorq ") + textStyle.Render("WORKSPACE STUDIO")
	header := fit(brand+"  "+dimStyle.Render("│ ")+cyanStyle.Render(branch)+"  "+dimStyle.Render(counts), m.width)
	path := m.dir
	if m.studio != nil {
		path = m.studio.Workspace.Path
	}
	subheader := fit(dimStyle.Render(" "+safeText(path)), m.width)
	treeTitle := " ARQUIVOS"
	if m.focus == "tree" {
		treeTitle = " ● ARQUIVOS"
	}
	fileTitle := " PREVIEW"
	if m.viewer.path != "" {
		fileTitle = " " + displayName(m.viewer.path)
	}
	mode := strings.ToUpper(m.viewer.mode)
	if mode != "" {
		fileTitle += "  [" + mode + "]"
	}
	if m.focus == "viewer" {
		fileTitle = " ●" + fileTitle
	}
	titles := fit(accentStyle.Render(treeTitle), left) + dimStyle.Render(" │ ") + fit(cyanStyle.Render(fileTitle), right)
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.treeView(left), dimStyle.Render(strings.TrimSuffix(strings.Repeat(" │ \n", m.bodyHeight()), "\n")), m.viewer.view(right, m.bodyHeight()))
	status := m.notice
	if m.loading {
		status = m.spinner() + " Atualizando workspace…"
	}
	if m.busy != "" {
		status = m.spinner() + " " + m.busy
	}
	if status == "" {
		status = "Pronto · Shift+Tab alterna foco · Tab alterna diff/arquivo"
	}
	notice := fit(dimStyle.Render(" "+safeText(status)), m.width)
	if strings.HasPrefix(status, "Erro:") || strings.HasPrefix(status, "Commit:") || strings.HasPrefix(status, "Push:") || strings.HasPrefix(status, "Worktrees:") {
		notice = fit(redStyle.Render(" "+safeText(status)), m.width)
	}
	footer := fit(accentStyle.Render(" [s] Stage  [c] Commit  [p] Push  [w] Worktrees ")+dimStyle.Render(" [r] Atualizar  [?] Ajuda  [q] Sair"), m.width)
	if m.width < 95 {
		footer = fit(accentStyle.Render(" [s] Stage [c] Commit [p] Push [w] WT ")+dimStyle.Render(" [?] Ajuda [q] Sair"), m.width)
	}
	if m.width < 60 {
		footer = fit(accentStyle.Render(" [s] Stage [c] Commit [p] Push [w] WT [q] Sair"), m.width)
	}
	base := strings.Join([]string{header, subheader, titles, body, notice, footer}, "\n")
	if m.modal != "" {
		return overlay(base, m.modalView(), m.width, m.height)
	}
	return base
}

func (m *Model) spinner() string { return []string{"◐", "◓", "◑", "◒"}[m.frame%4] }

func (m *Model) modalView() string {
	width := min(76, m.width-8)
	var title, body, help string
	switch m.modal {
	case "help":
		title = "ATALHOS · WORKSPACE STUDIO"
		body = "Árvore   ↑/↓ ou j/k navegar · Enter/Espaço expandir\n         → ou Enter em arquivo foca o preview\nPreview  ↑/↓ ou j/k rolar · PgUp/PgDown página\n         ←/→ rolagem horizontal · Home/End extremos\nTab      alternar Diff / Arquivo completo\nShift+Tab alternar foco entre árvore e preview\ns        stage / unstage do arquivo selecionado\nc        commit guiado com edição da mensagem\np        push explícito com destino visível\nw        listar e alternar worktrees (sem checkout)\nr        atualizar · i alternar ícones Nerd Font\nq / Ctrl+C sair · atualização automática a cada 5s"
		help = "Esc / ? fecha a ajuda"
	case "commit":
		title = "COMMIT LOCAL"
		switch m.commitPhase {
		case "type":
			body = "Escolha o tipo do Conventional Commit:\n\n[1] feat      [2] fix       [3] chore      [4] refactor\n[5] docs      [6] test      [7] style      [8] perf"
			help = "1–8 escolher · Esc cancelar"
		case "loading":
			body = "Gerando proposta a partir do stage…"
			help = "Esc cancelar"
		case "edit":
			start := max(0, m.cursor-max(1, width-10))
			body = "Edite a mensagem:\n\n" + safeText(string(m.edit[start:m.cursor])) + "▏" + safeText(string(m.edit[m.cursor:]))
			help = "Enter revisar · Esc voltar · Ctrl+U limpar"
		default:
			body = fmt.Sprintf("%d arquivo(s) em stage\n\n%s\n\nO commit permanece local. Nenhum push automático.", m.commitCount, safeText(m.commitMessage))
			help = "Enter confirmar · e editar · Esc cancelar (preserva stage)"
		}
	case "push":
		title = "PUBLICAR COMMITS"
		if m.pushTarget == nil {
			body = "Inspecionando destino…"
		} else {
			t := m.pushTarget
			body = fmt.Sprintf("Local: %s -> Remote: %s/%s", safeText(t.Branch), safeText(t.Remote), safeText(t.RemoteBranch))
			if t.HasUpstream {
				if t.Ahead >= 0 {
					body += fmt.Sprintf("\n\n%d commit(s) à frente · %d atrás", t.Ahead, t.Behind)
				}
			} else {
				body += "\n\nSem upstream. Publicar e configurar upstream nesta branch?"
			}
			if t.Behind > 0 {
				body += "\nO Git recusará divergências; nenhum force será aplicado."
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
					state = fmt.Sprintf("  +%d ~%d ?%d", w.Status.StagedCount(), w.Status.UnstagedCount(), w.Status.UntrackedCount())
				}
				if w.IsBare {
					state = "  [bare]"
				}
				rows = append(rows, mark+displayName(w.Branch)+state, "  "+safeText(w.Path))
			}
			body = strings.Join(rows, "\n")
		}
		help = "↑/↓ escolher · Enter abrir · Esc cancelar"
	}
	if m.busy != "" {
		help = m.spinner() + " " + m.busy + " · Esc fecha o diálogo"
	}
	inner := max(1, width-4)
	lines := []string{accentStyle.Render(title), ""}
	for _, line := range strings.Split(safeText(body), "\n") {
		wrapped := strings.Split(lipgloss.NewStyle().Width(inner).Render(line), "\n")
		for _, part := range wrapped {
			lines = append(lines, fit(part, inner))
		}
	}
	limit := max(1, m.height-8)
	if len(lines) > limit {
		lines = append(lines[:limit-1], dimStyle.Render("…"))
	}
	lines = append(lines, "", cyanStyle.Render(fit(safeText(help), inner)))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1).Width(width - 2).Render(strings.Join(lines, "\n"))
}
