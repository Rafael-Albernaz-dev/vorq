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
		m.modal = modalUnstage
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
	m.reviewGuard, m.guardOverride = nil, false
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
	m.rightTab = "commit"
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
	m.modal = modalCommit
	m.modalScroll = 0
	m.commitPhase = "loading"
	m.busy = "Revisando arquivos no stage…"
	m.commitEditing = false
	ctx, cancel := m.beginDialog(20 * time.Second)
	return func() tea.Msg {
		defer cancel()
		cs, err := core.GetCommitChanges(ctx, dir)
		var guard *domain.GuardReport
		if err == nil {
			guard, err = core.CheckCommit(ctx, dir)
		}
		return reviewMsg{id: id, revision: revision, changes: cs, guard: guard, err: err}
	}
}

func (m *Model) commit() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	if m.reviewGuard.HasBlocking() && !m.guardOverride {
		m.openGuard(modalCommit)
		return nil
	}
	op, ctx := m.commitOp(), m.ctx
	return func() tea.Msg { return op(ctx) }
}

// commitOp creates the reviewed commit; the core rejects a changed stage.
func (m *Model) commitOp() operation {
	m.dialogID++
	id, core, dir, message, diff, allow := m.dialogID, m.core, m.dir, m.reviewMessage, m.reviewDiff, m.guardOverride
	m.busy = "Criando commit local…"
	m.mutating = true
	return func(ctx context.Context) tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := core.CreateReviewedCommit(ctx, dir, message, diff, allow)
		if err != nil {
			return actionMsg{id: id, kind: "commit", err: guardError(err), created: commitWasCreated(result, err)}
		}
		notice := fmt.Sprintf("Commit local %s · %s", result.ShortHash, result.Subject)
		return actionMsg{id: id, kind: "commit", notice: notice + guardResultNote(result.Overridden, result.Warnings)}
	}
}

func (m *Model) inspectPush() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = modalPush
	m.modalScroll = 0
	m.pushTarget, m.guardOverride = nil, false
	m.busy = "Avaliando destino e commits não publicados…"
	ctx, cancel := m.beginDialog(60 * time.Second)
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
	if m.pushTarget.Guard.HasBlocking() && !m.guardOverride {
		m.openGuard(modalPush)
		return nil
	}
	op, ctx := m.pushOp(), m.ctx
	return func() tea.Msg { return op(ctx) }
}

// pushOp publishes to the reviewed target; the core revalidates it.
func (m *Model) pushOp() operation {
	m.dialogID++
	id, core, dir, target, allow := m.dialogID, m.core, m.dir, *m.pushTarget, m.guardOverride
	m.busy = "Publicando no destino confirmado…"
	m.mutating = true
	return func(ctx context.Context) tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := core.PushChanges(ctx, dir, domain.PushOptions{Target: target, AllowFindings: allow})
		if err != nil {
			return actionMsg{id: id, kind: "push", err: guardError(err)}
		}
		notice := "Push concluído: " + result.Target.Remote + "/" + result.Target.RemoteBranch
		return actionMsg{id: id, kind: "push", notice: notice + guardResultNote(result.Overridden, nil)}
	}
}

func (m *Model) inspectWorktrees() tea.Cmd {
	if !m.canAct() {
		return nil
	}
	m.dialogID++
	id, core, dir := m.dialogID, m.core, m.dir
	m.modal = modalWorktrees
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
