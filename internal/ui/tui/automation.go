package tui

import (
	"fmt"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type automationMsg struct {
	id     uint64
	kind   string
	plan   *domain.CommitPlan
	review *domain.DiffReview
	err    error
}

func (m *Model) openAIAction(task string) tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.aiTask = task
	m.rightTab = "ai"
	m.commitEditing = false
	if task != "configure" && m.aiProvider != "" && len(m.aiModels) > 0 {
		m.modal, m.modalScroll = "ai-send", 0
		return nil
	}
	m.modal, m.modalScroll = "suggest", 0
	return nil
}
func (m *Model) generateAutomation() tea.Cmd {
	m.dialogID++
	id, dir, core, provider, model, task := m.dialogID, m.dir, m.core, m.aiProvider, m.aiModel, m.aiTask
	m.busy = "IA analisando mudanças elegíveis…"
	ctx, cancel := m.beginDialog(120 * time.Second)
	return func() tea.Msg {
		defer cancel()
		msg := automationMsg{id: id, kind: task}
		if task == "review" {
			msg.review, msg.err = core.ReviewDiffAI(ctx, dir, provider, model)
		} else {
			msg.plan, msg.err = core.PlanCommitsAI(ctx, dir, provider, model)
		}
		return msg
	}
}
func (m *Model) loadAutomationPlan() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "plan"
	m.busy = "Carregando plano salvo…"
	ctx, cancel := m.beginDialog(10 * time.Second)
	return func() tea.Msg {
		defer cancel()
		p, err := core.LoadCommitPlan(ctx, dir)
		return automationMsg{id: id, kind: "load", plan: p, err: err}
	}
}
func (m *Model) receiveAutomation(msg automationMsg) tea.Cmd {
	if msg.id != m.dialogID {
		return nil
	}
	m.busy = ""
	m.mutating = false
	if msg.review != nil {
		m.diffReview = msg.review
		m.modal, m.modalScroll = "diff-review", 0
	}
	if msg.plan != nil {
		m.commitPlan = msg.plan
		m.planGroup = min(m.planGroup, max(0, len(msg.plan.Commits)-1))
		m.modal, m.modalScroll = "plan", 0
	}
	if msg.err != nil {
		if msg.plan == nil {
			m.aiFailure(msg.err)
		} else {
			m.notice = "Plano interrompido: " + msg.err.Error()
		}
	} else if msg.kind == "execute" {
		m.notice = fmt.Sprintf("%d commits locais concluídos; nenhum push.", len(msg.plan.Completed))
	}
	if msg.kind == "execute" {
		return m.refresh()
	}
	return nil
}
func (m *Model) executeAutomationPlan() tea.Cmd {
	if !m.canAct() || m.commitPlan == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, plan, ctx := m.dialogID, m.core, m.dir, m.commitPlan, m.ctx
	m.busy = "Executando commits aprovados…"
	m.mutating = true
	return func() tea.Msg {
		p, err := core.ExecuteCommitPlan(ctx, dir, plan)
		return automationMsg{id: id, kind: "execute", plan: p, err: err}
	}
}
func (m *Model) automationKey(key tea.KeyMsg) tea.Cmd {
	s := key.String()
	if m.modal == "plan-edit" {
		if s == "esc" {
			m.modal = "plan"
			return nil
		}
		if m.busy != "" {
			return nil
		}
		switch s {
		case "ctrl+s":
			return m.saveAutomationEdit()
		case "backspace":
			r := []rune(m.planEditValue)
			if len(r) > 0 {
				m.planEditValue = string(r[:len(r)-1])
			}
		case "ctrl+u":
			m.planEditValue = ""
		case "enter":
			if m.planEditKind == "message" {
				m.planEditValue += "\n"
			} else {
				return m.saveAutomationEdit()
			}
		default:
			if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
				value := string(key.Runes)
				if key.Type == tea.KeySpace {
					value = " "
				}
				value = strings.Map(func(r rune) rune {
					if unicode.IsControl(r) {
						return -1
					}
					return r
				}, value)
				if len(m.planEditValue)+len(value) <= 8192 {
					m.planEditValue += value
				}
			}
		}
		return nil
	}
	if m.busy != "" {
		return nil
	}
	if m.modal == "plan-execute" {
		if s == "enter" {
			return m.executeAutomationPlan()
		}
		return nil
	}
	if m.modal == "diff-review" {
		if s == "o" {
			return m.openAIAction("plan")
		}
		return nil
	}
	if m.modal != "plan" || m.commitPlan == nil {
		return nil
	}
	switch s {
	case "up", "k":
		m.planGroup = max(0, m.planGroup-1)
		m.modalScroll = 0
	case "down", "j":
		m.planGroup = min(len(m.commitPlan.Commits)-1, m.planGroup+1)
		m.modalScroll = 0
	case "enter":
		m.modal, m.modalScroll = "plan-execute", 0
	case "d":
		m.modal, m.modalScroll = "plan-diff", 0
	case "e", "m", "x", "r":
		if len(m.commitPlan.Completed) > 0 {
			m.notice = "Execução iniciada: retome ou gere novo plano das mudanças restantes."
			return nil
		}
		m.planEditKind = map[string]string{"e": "message", "m": "move", "x": "exclude", "r": "order"}[s]
		m.planEditValue = ""
		if s == "e" {
			c := m.commitPlan.Commits[m.planGroup]
			m.planEditValue = c.Subject + "\n\n" + c.Body
		}
		m.modal, m.modalScroll = "plan-edit", 0
	}
	return nil
}
func (m *Model) saveAutomationEdit() tea.Cmd {
	if m.commitPlan == nil || m.busy != "" {
		return nil
	}
	edit := domain.PlanEdit{Kind: m.planEditKind, Group: m.planGroup}
	switch edit.Kind {
	case "message":
		parts := strings.SplitN(m.planEditValue, "\n", 2)
		edit.Subject = parts[0]
		if len(parts) > 1 {
			edit.Body = strings.TrimSpace(parts[1])
		}
	case "move":
		parts := strings.Fields(m.planEditValue)
		if len(parts) != 2 {
			m.notice = "Informe ID e grupo, exemplo F0001 2"
			return nil
		}
		edit.FileID = parts[0]
		target, err := strconv.Atoi(parts[1])
		if err != nil {
			m.notice = "Grupo inválido"
			return nil
		}
		edit.Target = target - 1
	case "exclude":
		edit.FileID = strings.TrimSpace(m.planEditValue)
	case "order":
		target, err := strconv.Atoi(strings.TrimSpace(m.planEditValue))
		if err != nil {
			m.notice = "Informe número do destino"
			return nil
		}
		edit.Target = target - 1
	}
	m.dialogID++
	id, core, dir, p := m.dialogID, m.core, m.dir, m.commitPlan
	m.busy = "Salvando plano…"
	ctx, cancel := m.beginDialog(10 * time.Second)
	return func() tea.Msg {
		defer cancel()
		updated, err := core.EditCommitPlan(ctx, dir, p, edit)
		if updated == nil {
			updated = p
		}
		return automationMsg{id: id, kind: "edit", plan: updated, err: err}
	}
}
func (m *Model) planBody() string {
	if m.commitPlan == nil {
		return "Nenhum plano carregado."
	}
	p := m.commitPlan
	var b strings.Builder
	fmt.Fprintf(&b, "Plano %s · %d/%d commits concluídos\n", p.ID[:min(12, len(p.ID))], len(p.Completed), len(p.Commits))
	for i, c := range p.Commits {
		mark := " "
		if i == m.planGroup {
			mark = ">"
		}
		fmt.Fprintf(&b, "%s %d. %s\n", mark, i+1, c.Subject)
		if i < len(p.Completed) {
			fmt.Fprintf(&b, "   Criado: %s\n", p.Completed[i].Hash)
		}
	}
	if len(p.Commits) > 0 {
		c := p.Commits[m.planGroup]
		fmt.Fprintf(&b, "\nGrupo %d: %s\n%s\nMotivo: %s\n", m.planGroup+1, c.Subject, c.Body, c.Reason)
		for _, id := range c.Files {
			for _, f := range p.Snapshot.Files {
				if f.ID == id {
					fmt.Fprintf(&b, "%s %s\n", id, f.Path)
				}
			}
		}
	}
	for _, f := range p.Snapshot.Files {
		if f.Excluded != "" {
			fmt.Fprintf(&b, "\nExcluído %s: %s", f.Path, f.Excluded)
		}
	}
	if p.Error != "" {
		b.WriteString("\n\nInterrupção: " + p.Error)
	}
	b.WriteString("\n\ne mensagem · m mover · x excluir · r ordenar · d diff\nArquivos inteiros: a versão de trabalho revisada será commitada.")
	return b.String()
}
func (m *Model) planDiffBody() string {
	if m.commitPlan == nil {
		return "Nenhum plano"
	}
	var b strings.Builder
	for _, id := range m.commitPlan.Commits[m.planGroup].Files {
		for _, f := range m.commitPlan.Snapshot.Files {
			if f.ID == id {
				b.WriteString(f.ID + " " + f.Path + "\n" + f.Diff + "\n\n")
			}
		}
	}
	return b.String()
}
func (m *Model) diffReviewBody() string {
	if m.diffReview == nil {
		return "Nenhuma revisão"
	}
	var b strings.Builder
	b.WriteString(m.diffReview.Summary + "\n")
	for _, finding := range m.diffReview.Findings {
		path := finding.FileID
		for _, f := range m.diffReview.Snapshot.Files {
			if f.ID == finding.FileID {
				path = f.Path
			}
		}
		fmt.Fprintf(&b, "\n[%s] %s\n%s\nEvidência: %s\n", finding.Severity, path, finding.Message, finding.Evidence)
	}
	for _, f := range m.diffReview.Snapshot.Files {
		if f.Excluded != "" {
			fmt.Fprintf(&b, "\nNão analisado %s: %s", f.Path, f.Excluded)
		}
	}
	b.WriteString("\n\nRevisão por IA; nenhum teste ou correção automática executado.")
	return b.String()
}
