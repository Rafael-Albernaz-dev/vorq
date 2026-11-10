package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

type prAI struct {
	aiFunc
	payload string
	title   string
	onCall  func()
}

func (p *prAI) Analyze(_ context.Context, r domain.AnalysisRequest) (json.RawMessage, error) {
	if r.Task != "pr" {
		return nil, nil
	}
	p.payload = r.Payload
	if p.onCall != nil {
		p.onCall()
	}
	return json.Marshal(map[string]string{"title": p.title, "body": "## Summary\nAdds the greeting.\n\n## How to test\nRun the binary."})
}

func prFixture(t *testing.T) (*app.App, *git.NativeRunner, string, *prAI) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	_, r, dir := seedWorkflow(t)
	ctx := context.Background()
	a := app.NewApp(&prRepository{Repository: git.NewRepository(r)})
	a.SetPullRequestProvider(&fakePRProvider{})
	if _, e := r.Exec(ctx, dir, "remote", "add", "origin", "git@github.com:owner/repo.git"); e != nil {
		t.Fatal(e)
	}
	writeCommitFile(t, dir, ".github/pull_request_template.md", "## What\n\n## Checklist\n- [ ] tests\n")
	writeCommitFile(t, dir, ".gitignore", "secret.env\n")
	_ = r.Add(ctx, dir, ".")
	_, _ = r.Commit(ctx, dir, "chore: template")
	_ = a.CreateBranch(ctx, dir, domain.BranchOptions{Name: "feature"})
	writeCommitFile(t, dir, "greet.go", "package main\n// greet\n")
	_ = r.Add(ctx, dir, "greet.go")
	_, _ = r.Commit(ctx, dir, "feat: add greeting")
	writeCommitFile(t, dir, "secret.env", "TOKEN=do-not-send\n")
	ai := &prAI{title: "feat: add greeting"}
	a.SetAIProvider(ai)
	return a, r, dir, ai
}

func TestDraftPullRequestAIUsesBranchDiffAndTemplate(t *testing.T) {
	a, _, dir, ai := prFixture(t)
	d, err := a.DraftPullRequestAI(context.Background(), dir, "", "claude", "sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "feat: add greeting" || !strings.Contains(d.Body, "How to test") || d.Head != "feature" || !d.Draft {
		t.Fatalf("draft %+v", d)
	}
	var sent struct {
		Commits []domain.BranchCommit    `json:"commits"`
		Files   []domain.BranchFileDiff  `json:"files"`
		Docs    []domain.ContextDocument `json:"context_documents"`
	}
	if err = json.Unmarshal([]byte(ai.payload), &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Commits) != 1 || sent.Commits[0].Subject != "feat: add greeting" {
		t.Fatalf("branch commits %+v (base commits must not be included)", sent.Commits)
	}
	if len(sent.Files) != 1 || sent.Files[0].Path != "greet.go" || !strings.Contains(sent.Files[0].Diff, "+// greet") {
		t.Fatalf("branch diff %+v", sent.Files)
	}
	template := false
	for _, doc := range sent.Docs {
		template = template || doc.Path == ".github/pull_request_template.md"
	}
	if !template {
		t.Fatal("PR template not sent as context")
	}
	if strings.Contains(ai.payload, "do-not-send") {
		t.Fatal("untracked/ignored working file leaked into the PR analysis")
	}
}

func TestDraftPullRequestAIRejectsMovedHeadAndBadText(t *testing.T) {
	a, r, dir, ai := prFixture(t)
	ctx := context.Background()
	ai.onCall = func() { _, _ = r.Exec(ctx, dir, "commit", "--allow-empty", "-m", "chore: moved") }
	if _, err := a.DraftPullRequestAI(ctx, dir, "", "claude", ""); err == nil || !strings.Contains(err.Error(), "HEAD changed") {
		t.Fatalf("moved HEAD must be refused: %v", err)
	}
	ai.onCall, ai.title = nil, "bad\ntitle"
	if _, err := a.DraftPullRequestAI(ctx, dir, "", "claude", ""); err == nil {
		t.Fatal("multi-line title accepted")
	}
	if _, err := a.DraftPullRequestAI(ctx, dir, "", "ollama", ""); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
