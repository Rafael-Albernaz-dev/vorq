package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) stage() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	node := m.selectedNode()
	if node == nil || node.IsDir {
		m.notice = "Selecione um arquivo para stage/unstage."
		return nil
	}
	if !node.Status.IsStaged() && !node.Status.IsUnstaged() && !node.Status.IsUntracked() && !node.Status.IsConflicted() {
		m.notice = "Arquivo sem alterações"
		return nil
	}
	if node.Status.IsStaged() && node.Status.IsUnstaged() && m.partialApproved != node.Path {
		m.modal = "unstage"
		m.modalScroll = 0
		return nil
	}
	m.partialApproved = ""
	m.dialogID++
	id, path, dir, core, ctx := m.dialogID, node.Path, m.dir, m.core, m.ctx
	m.busy = "Atualizando stage…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		err := core.ToggleStage(ctx, dir, path)
		return actionMsg{id: id, kind: "stage", notice: "Stage atualizado: " + path, err: err}
	}
}

func (m *Model) propose(commitType string) tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.commitType = commitType
	m.dialogID++
	id, revision := m.dialogID, m.draftRevision
	m.commitPhase = "loading"
	m.busy = "Gerando sugestão…"
	core, dir := m.core, m.dir
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		proposal, err := core.ProposeCommit(ctx, dir, commitType)
		return proposalMsg{id: id, revision: revision, proposal: proposal, err: err}
	}
}

func (m *Model) review() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	if m.changeSummary().Conflicts > 0 {
		m.notice = "Resolva os conflitos externamente e prepare a resolução antes de comitar."
		return nil
	}
	if strings.TrimSpace(m.commitSummary) == "" {
		m.notice = "Preencha o Resumo antes de confirmar o commit."
		return nil
	}
	if m.changeSummary().Staged == 0 {
		m.notice = "Nenhum arquivo no stage. Use s para preparar um arquivo."
		return nil
	}
	m.dialogID++
	id, revision, core, dir := m.dialogID, m.draftRevision, m.core, m.dir
	m.modal = "commit"
	m.modalScroll = 0
	m.commitPhase = "loading"
	m.busy = "Revisando arquivos no stage…"
	m.commitEditing = false
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		cs, err := core.GetCommitChanges(ctx, dir)
		return reviewMsg{id: id, revision: revision, changes: cs, err: err}
	}
}

func (m *Model) commit() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir, message, diff, ctx := m.dialogID, m.core, m.dir, m.reviewMessage, m.reviewDiff, m.ctx
	m.busy = "Criando commit local…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := core.CreateReviewedCommit(ctx, dir, message, diff)
		if err != nil {
			return actionMsg{id: id, kind: "commit", err: err, created: commitWasCreated(result, err)}
		}
		return actionMsg{id: id, kind: "commit", notice: fmt.Sprintf("Commit local %s · %s", result.ShortHash, result.Subject)}
	}
}

func (m *Model) inspectPush() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "push"
	m.modalScroll = 0
	m.pushTarget = nil
	m.busy = "Inspecionando destino…"
	ctx, cancel := m.beginDialog(15 * time.Second)
	return func() tea.Msg {
		defer cancel()
		target, err := core.GetPushTarget(ctx, dir)
		return pushTargetMsg{id: id, target: target, err: err}
	}
}

func (m *Model) push() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	if m.pushTarget == nil {
		return nil
	}
	m.dialogID++
	id, core, dir, target, ctx := m.dialogID, m.core, m.dir, *m.pushTarget, m.ctx
	m.busy = "Publicando no destino confirmado…"
	m.mutating = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := core.PushChanges(ctx, dir, domain.PushOptions{Target: target})
		if err != nil {
			return actionMsg{id: id, kind: "push", err: err}
		}
		return actionMsg{id: id, kind: "push", notice: "Push concluído: " + result.Target.Remote + "/" + result.Target.RemoteBranch}
	}
}

func (m *Model) inspectWorktrees() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = "worktrees"
	m.modalScroll = 0
	m.worktrees = nil
	m.busy = "Listando worktrees…"
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		worktrees, err := core.ListWorktrees(ctx, dir)
		return worktreesMsg{id: id, worktrees: worktrees, err: err}
	}
}

func commitWasCreated(result *domain.CommitResult, err error) bool {
	var outcome *domain.CommitMetadataError
	return (result != nil && result.Hash != "") || errors.As(err, &outcome)
}
