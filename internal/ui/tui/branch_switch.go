package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type branchesMsg struct {
	id       uint64
	branches []domain.LocalBranch
	err      error
}
type branchSwitchPlanMsg struct {
	id   uint64
	plan *domain.BranchSwitchPlan
	err  error
}

func (m *Model) inspectBranches() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "branches"
	m.modalScroll = 0
	m.localBranches = nil
	m.branchSwitchPlan = nil
	m.busy = "Listando branches locais…"
	m.commitEditing = false
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		branches, err := core.ListLocalBranches(ctx, dir)
		return branchesMsg{id, branches, err}
	}
}
func (m *Model) reviewBranchSwitch() tea.Cmd {
	if !m.canAct() || len(m.localBranches) == 0 {
		return nil
	}
	m.dialogID++
	id, core, dir, name := m.dialogID, m.core, m.dir, m.localBranches[m.branchIndex].Name
	m.modal = "switch-branch"
	m.modalScroll = 0
	m.branchSwitchPlan = nil
	m.busy = "Revisando troca de branch…"
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		plan, err := core.PrepareBranchSwitch(ctx, dir, name)
		return branchSwitchPlanMsg{id, plan, err}
	}
}
func (m *Model) switchBranch() tea.Cmd {
	if !m.canAct() || m.branchSwitchPlan == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, plan, ctx := m.dialogID, m.core, m.dir, *m.branchSwitchPlan, m.ctx
	m.busy = "Trocando branch…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := core.SwitchBranch(ctx, dir, plan)
		return actionMsg{id: id, kind: "switch-branch", notice: fmt.Sprintf("Branch ativa: %s", plan.Target), err: err}
	}
}
func (m *Model) branchSelectorKey(key tea.KeyMsg) tea.Cmd {
	if m.busy != "" {
		return nil
	}
	switch key.String() {
	case "up", "k":
		m.branchIndex = max(0, m.branchIndex-1)
	case "down", "j":
		m.branchIndex = min(max(0, len(m.localBranches)-1), m.branchIndex+1)
	case "pgup":
		m.branchIndex = max(0, m.branchIndex-m.modalBodyHeight())
	case "pgdown":
		m.branchIndex = min(max(0, len(m.localBranches)-1), m.branchIndex+m.modalBodyHeight())
	case "enter":
		return m.reviewBranchSwitch()
	}
	return nil
}
func (m *Model) branchModalRows() ([]string, []int) {
	width := min(90, m.width-6) - 4
	visible := max(1, m.modalBodyHeight())
	start := max(0, m.branchIndex-visible+1)
	var rows []string
	var owners []int
	for i := start; i < min(len(m.localBranches), start+visible); i++ {
		b := m.localBranches[i]
		mark := "  "
		if i == m.branchIndex {
			mark = "› "
		}
		flag := ""
		if b.Current {
			flag = " [atual]"
		} else if b.WorktreePath != "" {
			flag = " [outro worktree]"
		}
		nameWidth := max(1, width-ansi.StringWidth(mark+flag))
		rows = append(rows, mark+ansi.Truncate(safeText(b.Name), nameWidth, "…")+flag)
		owners = append(owners, i)
	}
	return rows, owners
}
func (m *Model) branchSelectorMouse(msg tea.MouseMsg) tea.Cmd {
	if msg.Button == tea.MouseButtonWheelUp {
		return m.branchSelectorKey(tea.KeyMsg{Type: tea.KeyUp})
	}
	if msg.Button == tea.MouseButtonWheelDown {
		return m.branchSelectorKey(tea.KeyMsg{Type: tea.KeyDown})
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
	if mouseOutlinedButtonHit(box, "[Voltar]", x, y) {
		return m.handleModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	}
	if m.busy != "" {
		return nil
	}
	if mouseOutlinedButtonHit(box, "[Revisar]", x, y) {
		return m.reviewBranchSwitch()
	}
	_, owners := m.branchModalRows()
	if y >= 4 && y < 4+len(owners) {
		m.branchIndex = owners[y-4]
	}
	return nil
}
