package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type PRMergeService interface {
	PreparePRMerge(context.Context, string, int) (*domain.PRMergePlan, error)
	MergePullRequest(context.Context, string, domain.PRMergePlan, string) (*domain.PRMergeResult, error)
}

// PreparePRMerge reads the PR and lists every reason GitHub or the policy
// (decision 4: failing/pending checks, review, draft, conflicts) blocks it.
func (a *App) PreparePRMerge(ctx context.Context, dir string, number int) (*domain.PRMergePlan, error) {
	if number <= 0 {
		return nil, fmt.Errorf("invalid PR number")
	}
	link, e := a.prRepository(ctx, dir)
	if e != nil {
		return nil, e
	}
	info, e := a.pullRequests.MergeInfo(ctx, dir, link.Slug, number)
	if e != nil {
		return nil, e
	}
	pr := info.PR
	if pr.Number != number || pr.HeadOID == "" {
		return nil, fmt.Errorf("GitHub returned an incomplete PR")
	}
	plan := &domain.PRMergePlan{Number: number, Repository: link.Slug, Title: pr.Title, Head: pr.Head, Base: pr.Base, HeadOID: pr.HeadOID, Methods: info.Methods}
	block := func(reason string) { plan.Blockers = append(plan.Blockers, reason) }
	if !strings.EqualFold(pr.State, "open") {
		block(fmt.Sprintf("o PR não está aberto (%s)", strings.ToLower(pr.State)))
	}
	if pr.Draft {
		block("o PR é rascunho; marque como pronto (y) antes")
	}
	switch strings.ToUpper(info.Mergeable) {
	case "CONFLICTING":
		block("conflitos com " + pr.Base + "; traga a base para a branch e resolva")
	case "UNKNOWN", "":
		block("o GitHub ainda está calculando se dá para mergear; atualize em instantes")
	}
	switch strings.ToUpper(pr.ReviewDecision) {
	case "CHANGES_REQUESTED":
		block("revisão pediu mudanças")
	case "REVIEW_REQUIRED":
		block("revisão aprovada é obrigatória e ainda falta")
	}
	failing, pending := 0, 0
	for _, c := range pr.Checks {
		plan.ChecksTotal++
		conclusion := strings.ToUpper(c.Conclusion + c.State)
		status := strings.ToUpper(c.Status)
		switch {
		case strings.Contains(conclusion, "FAIL") || strings.Contains(conclusion, "ERROR") || strings.Contains(conclusion, "CANCEL") || strings.Contains(conclusion, "TIMED_OUT") || strings.Contains(conclusion, "ACTION_REQUIRED"):
			failing++
		case (status != "" && status != "COMPLETED") || conclusion == "PENDING" || conclusion == "EXPECTED":
			pending++
		default:
			plan.ChecksPassed++
		}
	}
	if failing > 0 {
		block(fmt.Sprintf("%d check(s) falhando", failing))
	}
	if pending > 0 {
		block(fmt.Sprintf("%d check(s) em andamento", pending))
	}
	switch strings.ToUpper(info.MergeStateStatus) {
	case "BLOCKED":
		if len(plan.Blockers) == 0 {
			block("bloqueado pelas regras de proteção da branch " + pr.Base)
		}
	case "BEHIND":
		block("a branch está atrás de " + pr.Base + " e a proteção exige atualizar; use Trazer main e publique")
	}
	if len(plan.Methods) == 0 {
		block("o repositório não permite nenhum método de merge")
	}
	return plan, nil
}

// MergePullRequest re-reads the PR, requires the same head commit and no
// blockers, and merges with an allowed method. The branch is never deleted here.
func (a *App) MergePullRequest(ctx context.Context, dir string, reviewed domain.PRMergePlan, method string) (*domain.PRMergeResult, error) {
	plan, e := a.PreparePRMerge(ctx, dir, reviewed.Number)
	if e != nil {
		return nil, e
	}
	if plan.HeadOID != reviewed.HeadOID || plan.Repository != reviewed.Repository {
		return nil, fmt.Errorf("PR #%d changed after the review (new commits); review the merge again", plan.Number)
	}
	if len(plan.Blockers) > 0 {
		return nil, fmt.Errorf("PR #%d cannot be merged: %s", plan.Number, strings.Join(plan.Blockers, "; "))
	}
	if !slices.Contains(plan.Methods, method) {
		return nil, fmt.Errorf("merge method %q is not allowed in this repository (allowed: %s)", method, strings.Join(plan.Methods, ", "))
	}
	if e = a.pullRequests.Merge(ctx, dir, plan.Repository, plan.Number, method, plan.HeadOID); e != nil {
		return nil, e
	}
	return &domain.PRMergeResult{Plan: *plan, Method: method}, nil
}

// MergedHeadCovers reports whether deleting a branch only drops commits
// already merged through the PR: its local (and remote) tip is the merged head.
func MergedHeadCovers(p domain.BranchDeletePlan, mergedHead string) bool {
	return mergedHead != "" && p.Hash == mergedHead && (p.RemoteHash == "" || p.RemoteHash == mergedHead)
}
