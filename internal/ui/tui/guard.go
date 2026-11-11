package tui

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// VORQ-018: the Studio shows guardrail findings in the commit and push
// reviews; blocking findings need CONFIRMAR typed in a dedicated dialog.

var guardCategory = map[string]string{
	domain.GuardSecret:       "segredo",
	domain.GuardConflict:     "conflito",
	domain.GuardLargeFile:    "arquivo grande",
	domain.GuardDebug:        "debug",
	domain.GuardCaseConflict: "maiúsc./minúsc.",
	domain.GuardUnverified:   "não verificado",
}

func guardFindingLines(fs []domain.GuardFinding) []string {
	var lines []string
	for _, f := range fs {
		lines = append(lines, fmt.Sprintf("• [%s] %s — %s", guardCategory[f.Category], f.Location(), f.Message))
		if f.Excerpt != "" {
			lines = append(lines, "    "+f.Excerpt)
		}
	}
	return lines
}

// guardSummary is the findings section of a commit or push review.
func guardSummary(r *domain.GuardReport) string {
	if r == nil {
		return ""
	}
	if len(r.Findings) == 0 {
		return "\n\nVerificação de segurança: nenhum achado."
	}
	var b strings.Builder
	if blocking := r.Blocking(); len(blocking) > 0 {
		fmt.Fprintf(&b, "\n\nBLOQUEADO · %d achado(s) de segurança:\n", len(blocking))
		b.WriteString(strings.Join(guardFindingLines(blocking), "\n"))
		b.WriteString("\nConfirmar pedirá que você digite " + domain.GuardOverrideWord + ".")
	}
	if warnings := r.Warnings(); len(warnings) > 0 {
		fmt.Fprintf(&b, "\n\nAvisos (não bloqueiam) · %d:\n", len(warnings))
		b.WriteString(strings.Join(guardFindingLines(warnings), "\n"))
	}
	return b.String()
}

func (m *Model) guardReport() *domain.GuardReport {
	if m.guardFor == modalPush && m.pushTarget != nil {
		return m.pushTarget.Guard
	}
	return m.reviewGuard
}

func (m *Model) openGuard(from modalID) {
	m.guardFor, m.guardInput = from, ""
	m.modal, m.modalScroll = modalGuard, 0
}

func (m *Model) guardBody() string {
	r := m.guardReport()
	operation := "o commit"
	if m.guardFor == modalPush {
		operation = "o push"
	}
	body := fmt.Sprintf("%d achado(s) bloqueante(s). Valores sempre mascarados.\n\n", len(r.Blocking()))
	body += strings.Join(guardFindingLines(r.Blocking()), "\n")
	if m.guardFor == modalPush {
		body += "\n\nComo corrigir (o vorq não reescreve o histórico):"
		for _, step := range app.PushRemediation(r) {
			body += "\n→ " + step
		}
	} else {
		body += "\n\nCorrija os arquivos ou tire-os do stage. Falso positivo: comentário vorq:allow-secret na linha ou .vorqsecrets-allow."
	}
	body += fmt.Sprintf("\n\nO override vale só para %s revisado e fica registrado no resultado.\nDigite %s para seguir mesmo assim:\n\n> %s▏", operation, domain.GuardOverrideWord, m.guardInput)
	return body
}

func (m *Model) guardKey(key tea.KeyMsg) tea.Cmd {
	switch key.Type {
	case tea.KeyEsc:
		m.modal, m.guardInput, m.modalScroll = m.guardFor, "", 0
		m.notice = "Override não confirmado. Nada foi feito."
		return nil
	case tea.KeyEnter:
		if m.guardInput != domain.GuardOverrideWord {
			m.notice = "Digite exatamente " + domain.GuardOverrideWord + " (maiúsculas) ou Esc para voltar."
			return nil
		}
		m.guardOverride, m.guardInput = true, ""
		m.modal = m.guardFor
		if m.guardFor == modalPush {
			return m.push()
		}
		return m.commit()
	case tea.KeyBackspace:
		if r := []rune(m.guardInput); len(r) > 0 {
			m.guardInput = string(r[:len(r)-1])
		}
	case tea.KeyCtrlU:
		m.guardInput = ""
	case tea.KeyPgDown, tea.KeyDown:
		m.modalScroll++
	case tea.KeyPgUp, tea.KeyUp:
		m.modalScroll = max(0, m.modalScroll-1)
	case tea.KeyRunes:
		for _, r := range key.Runes {
			if unicode.IsPrint(r) && len(m.guardInput) < 32 {
				m.guardInput += string(r)
			}
		}
	}
	return nil
}

// guardError adds the masked findings to a block raised by the core itself
// (e.g. the stage or HEAD changed after the review).
func guardError(err error) error {
	var blocked *domain.GuardBlockedError
	if !errors.As(err, &blocked) {
		return err
	}
	return fmt.Errorf("%w\n\n%s", err, strings.Join(guardFindingLines(blocked.Report.Blocking()), "\n"))
}

func guardResultNote(overridden, warnings []domain.GuardFinding) string {
	note := ""
	if n := len(overridden); n > 0 {
		note += fmt.Sprintf(" · override registrado: %d achado(s) bloqueante(s) confirmados", n)
	}
	if n := len(warnings); n > 0 {
		note += fmt.Sprintf(" · %d aviso(s) de segurança", n)
	}
	return note
}
