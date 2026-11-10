package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// VORQ-015 Studio flow: IA tab → "Preparar próximo commit" → provider/model →
// disclosure → advice (PREPARAR / DEIXAR DE FORA) → [Aplicar] → confirmation
// listing every index change → stage updated and the suggested message put
// into the draft. Working files are never modified; nothing is committed.

const applyButton = "[Aplicar]"

// isWorkingAnalysis: tasks that analyse working versions (staged or not).
func isWorkingAnalysis(task aiTaskID) bool {
	return task == taskReview || task == taskPlan || task == taskStage
}

func (m *Model) stageAdviceKey(key tea.KeyMsg) tea.Cmd {
	s := key.String()
	switch m.modal {
	case modalStageAdvice:
		if s == "enter" || s == "a" {
			if m.stageAdvice == nil || len(m.stageAdvice.Stage) == 0 {
				m.notice = "A IA não recomendou nenhum arquivo para o próximo commit."
				return nil
			}
			m.modal, m.modalScroll = modalStageConfirm, 0
		}
	case modalStageConfirm:
		if s == "enter" {
			return m.applyStageAdvice()
		}
	}
	return nil
}

func (m *Model) applyStageAdvice() tea.Cmd {
	if !m.canAct() || m.stageAdvice == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, advice, parent := m.dialogID, m.core, m.dir, m.stageAdvice, m.ctx
	m.busy, m.mutating = "Aplicando o stage recomendado…", true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		err := core.ApplyStageAdvice(ctx, dir, advice)
		return actionMsg{id: id, kind: "stage-advice", notice: "Stage igual à recomendação da IA; revise a mensagem e o commit.", err: err}
	}
}

// receiveStageApplied fills the draft with the advised message after the
// index was updated (the confirmation already announced the replacement).
func (m *Model) receiveStageApplied() {
	a := m.stageAdvice
	m.stageAdvice = nil
	if a == nil || a.Subject == "" {
		return
	}
	m.commitSummary, m.commitDescription = a.Subject, a.Body
	m.summaryCursor, m.descriptionCursor = len([]rune(a.Subject)), len([]rune(a.Body))
	m.descriptionOffset = 0
	m.draftRevision++
	m.suggested = true
	m.suggestionSource = "IA: " + m.aiProvider + " / " + m.aiModelLabel()
	m.rightTab, m.focusPane, m.commitField, m.commitEditing = "commit", "commit", "summary", false
}

func (m *Model) adviceItemLines(items []domain.StageAdviceItem, width int, mark string, hold bool) []string {
	var lines []string
	for _, item := range items {
		path := safeText(m.adviceFilePath(item.FileID))
		lines = append(lines, "  "+mark+" "+cyanStyle.Bold(true).Render(path))
		lines = append(lines, wrapBlock(item.Reason, "      ", width, plain)...)
		if hold && strings.TrimSpace(item.Risk) != "" {
			lines = append(lines, wrapBlock("Risco: "+item.Risk, "      ", width, func(s string) string { return yellowStyle.Render(s) })...)
		}
		lines = append(lines, "")
	}
	return lines
}

func (m *Model) adviceFilePath(id string) string {
	if m.stageAdvice != nil {
		for _, f := range m.stageAdvice.Snapshot.Files {
			if f.ID == id {
				return f.Path
			}
		}
	}
	return id
}

// stageAdviceLines renders the advice in sections, like the review.
func (m *Model) stageAdviceLines(width int) []string {
	a := m.stageAdvice
	if a == nil {
		return []string{dimStyle.Render("Nenhuma recomendação")}
	}
	rule := dimStyle.Render(strings.Repeat("─", width))
	lines := []string{accentStyle.Render("RESUMO")}
	lines = append(lines, wrapBlock(a.Summary, "  ", width, plain)...)
	stats := []string{greenStyle.Render(fmt.Sprintf("%d para preparar", len(a.Stage))), yellowStyle.Render(fmt.Sprintf("%d fora deste commit", len(a.Hold)))}
	if a.Dropped > 0 {
		stats = append(stats, dimStyle.Render(fmt.Sprintf("%d referência(s) inválida(s) descartada(s)", a.Dropped)))
	}
	lines = append(lines, "", "  "+strings.Join(stats, dimStyle.Render("  ·  ")))
	context := "nenhum documento de contexto versionado encontrado"
	if len(a.Context) > 0 {
		context = strings.Join(a.Context, ", ")
	}
	lines = append(lines, wrapBlock("Contexto lido: "+context, "  ", width, func(s string) string { return dimStyle.Render(s) })...)
	lines = append(lines, "", rule, greenStyle.Bold(true).Render("PREPARAR NO PRÓXIMO COMMIT"))
	if len(a.Stage) == 0 {
		lines = append(lines, "  "+dimStyle.Render("Nenhum arquivo recomendado."), "")
	}
	lines = append(lines, m.adviceItemLines(a.Stage, width, greenStyle.Render("+"), false)...)
	lines = append(lines, rule, yellowStyle.Bold(true).Render("DEIXAR DE FORA"))
	if len(a.Hold) == 0 {
		lines = append(lines, "  "+dimStyle.Render("Nada fica de fora."), "")
	}
	lines = append(lines, m.adviceItemLines(a.Hold, width, yellowStyle.Render("−"), true)...)
	lines = append(lines, rule, accentStyle.Render("MENSAGEM SUGERIDA"))
	if a.Subject == "" {
		lines = append(lines, "  "+dimStyle.Render("Sem mensagem válida; escreva ou use Sugerir mensagem depois."))
	} else {
		lines = append(lines, wrapBlock(a.Subject, "  ", width, func(s string) string { return cyanStyle.Bold(true).Render(s) })...)
		lines = append(lines, wrapBlock(a.Body, "  ", width, plain)...)
	}
	var excluded []string
	for _, f := range a.Snapshot.Files {
		if f.Excluded != "" {
			excluded = append(excluded, f.Path+" — "+f.Excluded)
		}
	}
	if len(excluded) > 0 {
		lines = append(lines, "", rule, accentStyle.Render("NÃO ANALISADOS (não serão tocados)"))
		for _, e := range excluded {
			lines = append(lines, wrapBlock(e, "  • ", width, func(s string) string { return dimStyle.Render(s) })...)
		}
	}
	return lines
}

// stageConfirmLines lists every index change before anything is applied.
func (m *Model) stageConfirmLines(width int) []string {
	a := m.stageAdvice
	if a == nil {
		return nil
	}
	lines := wrapBlock("O stage ficará exatamente igual à recomendação. Somente o índice muda: nenhum arquivo de trabalho é alterado e nada é commitado ou publicado.", "", width, plain)
	lines = append(lines, "")
	if len(a.Changes) == 0 {
		lines = append(lines, "  "+dimStyle.Render("O stage já está igual à recomendação; nada a mudar no índice."))
	}
	for _, c := range a.Changes {
		label, style := "entra no stage", greenStyle
		if c.Action == "unstage" {
			label, style = "sai do stage  ", yellowStyle
		}
		path := c.Path
		if c.OldPath != "" && c.OldPath != c.Path {
			path = c.OldPath + " → " + c.Path
		}
		lines = append(lines, "  "+style.Render(label)+"  "+cyanStyle.Render(safeText(path)))
		if c.Partial {
			note := "estava parcialmente no stage: o arquivo inteiro passa a valer a versão de trabalho"
			if c.Action == "unstage" {
				note = "estava parcialmente no stage: a parte preparada sai do stage (o arquivo continua igual)"
			}
			lines = append(lines, wrapBlock(note, "      ", width, func(s string) string { return dimStyle.Render(s) })...)
		}
	}
	if a.Subject != "" {
		lines = append(lines, "")
		if strings.TrimSpace(m.commitSummary) != "" || strings.TrimSpace(m.commitDescription) != "" {
			lines = append(lines, wrapBlock("Seu rascunho atual será substituído pela mensagem sugerida.", "", width, func(s string) string { return yellowStyle.Render(s) })...)
		} else {
			lines = append(lines, wrapBlock("A mensagem sugerida irá para o editor de commit para você revisar.", "", width, plain)...)
		}
	}
	return lines
}
