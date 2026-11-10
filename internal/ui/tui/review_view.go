package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/charmbracelet/x/ansi"
)

// Structured layout for the AI review (post-VORQ-014 QA): a summary section,
// a severity count line, one separated card per finding (severity badge, file,
// message, quoted evidence, suggested fix) and the files left out. Provider
// text is untrusted: every piece goes through safeText before being styled.

var severityRank = map[string]int{"error": 0, "warning": 1, "info": 2}

func severityBadge(severity string) string {
	switch severity {
	case "error":
		return redStyle.Bold(true).Render("● ERRO")
	case "warning":
		return yellowStyle.Bold(true).Render("▲ AVISO")
	default:
		return cyanStyle.Bold(true).Render("ℹ INFO")
	}
}

// wrapBlock wraps untrusted text to width, prefixing every row and applying
// style to the text only (the prefix keeps its own style).
func wrapBlock(text, prefix string, width int, style func(string) string) []string {
	text = strings.TrimSpace(safeText(text))
	if text == "" {
		return nil
	}
	inner := max(1, width-ansi.StringWidth(ansi.Strip(prefix)))
	var rows []string
	for _, paragraph := range strings.Split(text, "\n") {
		if strings.TrimSpace(paragraph) == "" {
			rows = append(rows, "")
			continue
		}
		for _, row := range strings.Split(editorWrap(paragraph, inner), "\n") {
			rows = append(rows, prefix+style(row))
		}
	}
	return rows
}

func plain(s string) string { return s }

func (m *Model) reviewFilePath(r *domain.DiffReview, id string) string {
	for _, f := range r.Snapshot.Files {
		if f.ID == id {
			return f.Path
		}
	}
	return id
}

// diffReviewLines renders the review already wrapped to width.
func (m *Model) diffReviewLines(width int) []string {
	r := m.diffReview
	if r == nil {
		return []string{dimStyle.Render("Nenhuma revisão")}
	}
	rule := dimStyle.Render(strings.Repeat("─", width))
	section := func(title string) string { return accentStyle.Render(title) }
	lines := []string{section("RESUMO")}
	lines = append(lines, wrapBlock(r.Summary, "  ", width, plain)...)

	counts := map[string]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	var excluded []domain.SnapshotFile
	for _, f := range r.Snapshot.Files {
		if f.Excluded != "" {
			excluded = append(excluded, f)
		}
	}
	stats := []string{
		redStyle.Render(fmt.Sprintf("%d erro(s)", counts["error"])),
		yellowStyle.Render(fmt.Sprintf("%d aviso(s)", counts["warning"])),
		cyanStyle.Render(fmt.Sprintf("%d info", counts["info"])),
	}
	if r.Dropped > 0 {
		stats = append(stats, dimStyle.Render(fmt.Sprintf("%d descartado(s) sem evidência válida", r.Dropped)))
	}
	if len(excluded) > 0 {
		stats = append(stats, dimStyle.Render(fmt.Sprintf("%d arquivo(s) não analisado(s)", len(excluded))))
	}
	lines = append(lines, "", "  "+strings.Join(stats, dimStyle.Render("  ·  ")), "")

	findings := append([]domain.ReviewFinding(nil), r.Findings...)
	sort.SliceStable(findings, func(i, j int) bool {
		return severityRank[findings[i].Severity] < severityRank[findings[j].Severity]
	})
	if len(findings) == 0 {
		lines = append(lines, rule, "  "+greenStyle.Render("✓ Nenhum problema concreto encontrado."))
	}
	for i, f := range findings {
		lines = append(lines, rule)
		header := fmt.Sprintf(" %s  %s  %s", severityBadge(f.Severity), dimStyle.Render(fmt.Sprintf("%d/%d", i+1, len(findings))), cyanStyle.Bold(true).Render(safeText(m.reviewFilePath(r, f.FileID))))
		lines = append(lines, ansi.Truncate(header, width, "…"), "")
		lines = append(lines, wrapBlock(f.Message, "  ", width, plain)...)
		if evidence := wrapBlock(f.Evidence, "  "+dimStyle.Render("│ "), width, func(s string) string { return purpleStyle.Render(s) }); len(evidence) > 0 {
			lines = append(lines, "", "  "+dimStyle.Render("Evidência"))
			lines = append(lines, evidence...)
		}
		if suggestion := wrapBlock(f.Suggestion, "  "+greenStyle.Render("→ "), width, plain); len(suggestion) > 0 {
			lines = append(lines, "", "  "+greenStyle.Render("Sugestão"))
			lines = append(lines, suggestion...)
		}
		lines = append(lines, "")
	}
	if len(excluded) > 0 {
		lines = append(lines, rule, section("NÃO ANALISADOS"))
		for _, f := range excluded {
			lines = append(lines, wrapBlock(f.Path+" — "+f.Excluded, "  • ", width, func(s string) string { return dimStyle.Render(s) })...)
		}
	}
	return lines
}
