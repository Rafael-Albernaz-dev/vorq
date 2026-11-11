package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func mergeInfo(mutate func(*domain.PRMergeInfo)) *domain.PRMergeInfo {
	info := &domain.PRMergeInfo{Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN", Methods: []string{"merge", "squash"}}
	info.PR = domain.PullRequest{Number: 7, Title: "feat: x", State: "OPEN", Head: "feat/x", Base: "main", HeadOID: "abc123", ReviewDecision: "APPROVED"}
	info.PR.Checks = append(info.PR.Checks, struct {
		Name       string `json:"name"`
		Context    string `json:"context"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		State      string `json:"state"`
	}{Name: "ci", Status: "COMPLETED", Conclusion: "SUCCESS"})
	if mutate != nil {
		mutate(info)
	}
	return info
}

func setupPRMerge(t *testing.T, info *domain.PRMergeInfo) (*app.App, *fakePRProvider, string) {
	t.Helper()
	_, r, dir := seedWorkflow(t)
	a := app.NewApp(&prRepository{Repository: git.NewRepository(r)})
	provider := &fakePRProvider{info: info}
	a.SetPullRequestProvider(provider)
	if _, e := r.Exec(context.Background(), dir, "remote", "add", "origin", "git@github.com:owner/repo.git"); e != nil {
		t.Fatal(e)
	}
	return a, provider, dir
}

func TestPRMergePinnedToReviewedHead(t *testing.T) {
	a, provider, dir := setupPRMerge(t, mergeInfo(nil))
	ctx := context.Background()
	plan, err := a.PreparePRMerge(ctx, dir, 7)
	if err != nil || len(plan.Blockers) != 0 || plan.ChecksPassed != 1 || plan.HeadOID != "abc123" {
		t.Fatalf("plan %+v %v", plan, err)
	}
	if _, err := a.MergePullRequest(ctx, dir, *plan, "rebase"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("disallowed method: %v", err)
	}
	provider.info.PR.HeadOID = "def456"
	if _, err := a.MergePullRequest(ctx, dir, *plan, "squash"); err == nil || !strings.Contains(err.Error(), "changed after the review") {
		t.Fatalf("new head: %v", err)
	}
	provider.info.PR.HeadOID = "abc123"
	if _, err := a.MergePullRequest(ctx, dir, *plan, "squash"); err != nil {
		t.Fatal(err)
	}
	if len(provider.merged) != 1 || provider.merged[0] != "owner/repo#7 squash abc123" {
		t.Fatalf("merged %v", provider.merged)
	}
}

func TestPRMergeBlockers(t *testing.T) {
	cases := map[string]func(*domain.PRMergeInfo){
		"rascunho":        func(i *domain.PRMergeInfo) { i.PR.Draft = true },
		"conflitos":       func(i *domain.PRMergeInfo) { i.Mergeable = "CONFLICTING" },
		"calculando":      func(i *domain.PRMergeInfo) { i.Mergeable = "UNKNOWN" },
		"pediu mudanças":  func(i *domain.PRMergeInfo) { i.PR.ReviewDecision = "CHANGES_REQUESTED" },
		"falhando":        func(i *domain.PRMergeInfo) { i.PR.Checks[0].Conclusion = "FAILURE" },
		"em andamento":    func(i *domain.PRMergeInfo) { i.PR.Checks[0].Status = "IN_PROGRESS"; i.PR.Checks[0].Conclusion = "" },
		"não está aberto": func(i *domain.PRMergeInfo) { i.PR.State = "MERGED" },
		"proteção":        func(i *domain.PRMergeInfo) { i.MergeStateStatus = "BLOCKED" },
		"está atrás":      func(i *domain.PRMergeInfo) { i.MergeStateStatus = "BEHIND" },
		"nenhum método":   func(i *domain.PRMergeInfo) { i.Methods = nil },
	}
	for want, mutate := range cases {
		t.Run(want, func(t *testing.T) {
			a, provider, dir := setupPRMerge(t, mergeInfo(mutate))
			plan, err := a.PreparePRMerge(context.Background(), dir, 7)
			if err != nil || len(plan.Blockers) == 0 || !strings.Contains(strings.Join(plan.Blockers, "|"), want) {
				t.Fatalf("plan %+v %v", plan, err)
			}
			if _, err := a.MergePullRequest(context.Background(), dir, *plan, "merge"); err == nil || len(provider.merged) != 0 {
				t.Fatalf("blocked PR merged: %v", err)
			}
		})
	}
}

func TestMergedHeadCovers(t *testing.T) {
	p := domain.BranchDeletePlan{Hash: "abc", RemoteHash: "abc"}
	if !app.MergedHeadCovers(p, "abc") || app.MergedHeadCovers(p, "") || app.MergedHeadCovers(domain.BranchDeletePlan{Hash: "abc", RemoteHash: "new"}, "abc") {
		t.Fatal("coverage")
	}
}
