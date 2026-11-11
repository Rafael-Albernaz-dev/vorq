package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

var categoryLabel = map[string]string{
	domain.GuardSecret:       "segredo",
	domain.GuardConflict:     "conflito",
	domain.GuardLargeFile:    "arquivo grande",
	domain.GuardDebug:        "debug",
	domain.GuardCaseConflict: "maiúsc./minúsc.",
	domain.GuardUnverified:   "não verificado",
}

// printGuard lists findings, always masked, blocking first.
func printGuard(out io.Writer, report *domain.GuardReport) {
	if report == nil || len(report.Findings) == 0 {
		return
	}
	if b := report.Blocking(); len(b) > 0 {
		fmt.Fprintln(out, errorStyle.Render(fmt.Sprintf("🛑 %d achado(s) bloqueante(s):", len(b))))
		for _, f := range b {
			printFinding(out, f)
		}
	}
	if w := report.Warnings(); len(w) > 0 {
		fmt.Fprintln(out, warnStyle.Render(fmt.Sprintf("⚠ %d aviso(s) (não bloqueiam):", len(w))))
		for _, f := range w {
			printFinding(out, f)
		}
	}
}

func printFinding(out io.Writer, f domain.GuardFinding) {
	line := fmt.Sprintf("  • [%s] %s — %s (%s)", categoryLabel[f.Category], f.Location(), f.Message, f.Rule)
	fmt.Fprintln(out, strings.TrimRight(line, " "))
	if f.Excerpt != "" {
		fmt.Fprintln(out, subtleStyle.Render("      "+f.Excerpt))
	}
}

// guardGate shows the report and decides the override. Blocking findings
// stop unless --allow-secrets plus a typed CONFIRMAR (or --yes in scripts).
func guardGate(out io.Writer, reader *bufio.Reader, report *domain.GuardReport, operation string, allow, yes bool) (bool, error) {
	printGuard(out, report)
	if !report.HasBlocking() {
		return false, nil
	}
	blocked := &domain.GuardBlockedError{Operation: operation, Report: *report}
	if !allow {
		printRemediation(out, operation, report)
		fmt.Fprintln(out, subtleStyle.Render("Para seguir mesmo assim: --allow-secrets (confirmação digitada)."))
		return false, blocked
	}
	if yes {
		fmt.Fprintln(out, warnStyle.Render("Override confirmado por --allow-secrets --yes."))
		return true, nil
	}
	fmt.Fprintf(out, "Digite %s para fazer o %s mesmo assim (outra resposta cancela): ", domain.GuardOverrideWord, operation)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("override confirmation interrupted; nothing done: %w", err)
	}
	if strings.TrimSpace(line) != domain.GuardOverrideWord {
		fmt.Fprintln(out, subtleStyle.Render("Override não confirmado. Nada foi feito."))
		return false, blocked
	}
	return true, nil
}

func printRemediation(out io.Writer, operation string, report *domain.GuardReport) {
	if operation != "push" {
		fmt.Fprintln(out, subtleStyle.Render("Corrija os arquivos (ou tire do stage com git restore --staged <arquivo>) e tente de novo."))
		return
	}
	for _, step := range app.PushRemediation(report) {
		fmt.Fprintln(out, subtleStyle.Render("  → "+step))
	}
}

// printBlocked explains a block raised by the core itself (e.g. the stage
// changed between the check and the commit).
func printBlocked(out io.Writer, err error) {
	var blocked *domain.GuardBlockedError
	if errors.As(err, &blocked) {
		printGuard(out, &blocked.Report)
	}
}
