package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

// stageAI records the payload and answers with paths (as models sometimes do).
type stageAI struct {
	aiFunc
	payload string
	stage   []string
	hold    []string
}

func (s *stageAI) Analyze(_ context.Context, r domain.AnalysisRequest) (json.RawMessage, error) {
	s.payload = r.Payload
	item := func(paths []string) []map[string]string {
		var out []map[string]string
		for _, p := range paths {
			out = append(out, map[string]string{"file_id": p, "reason": "because " + p, "risk": "risk " + p})
		}
		return out
	}
	return json.Marshal(map[string]any{"summary": "Two cohesive changes", "stage": item(s.stage), "hold": item(s.hold), "subject": "feat: update a and add new file", "body": "Update a.txt and add new.txt."})
}

func stageFixture(t *testing.T) (*app.App, *git.NativeRunner, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, ".gitignore", "AGENTS.md\n")
	writeCommitFile(t, dir, "README.md", "# Project\nUse Conventional Commits.\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "docs/guide.md", "Every feature needs a test.\n")
	for _, f := range []string{"a.txt", "b.txt"} {
		writeCommitFile(t, dir, f, "base\n")
	}
	if err := r.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "chore: baseline"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "AGENTS.md", "PRIVATE agent instructions\n")  // ignored
	writeCommitFile(t, dir, "CONTRIBUTING.md", "untracked draft guide\n") // untracked
	writeCommitFile(t, dir, "a.txt", "changed in working tree\n")
	writeCommitFile(t, dir, "b.txt", "staged but unrelated\n")
	if err := r.Add(ctx, dir, "b.txt"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "new.txt", "new\n")
	return a, r, dir
}

func TestStageAdviceSendsOnlyTrackedContextAndAppliesExactly(t *testing.T) {
	a, r, dir := stageFixture(t)
	ctx := context.Background()
	ai := &stageAI{stage: []string{"a.txt", "new.txt"}, hold: []string{"b.txt", "CONTRIBUTING.md"}}
	a.SetAIProvider(ai)
	advice, err := a.StageAdviceAI(ctx, dir, "claude", "haiku")
	if err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Context []domain.ContextDocument `json:"context_documents"`
	}
	if err := json.Unmarshal([]byte(ai.payload), &sent); err != nil {
		t.Fatal(err)
	}
	var docs []string
	for _, d := range sent.Context {
		docs = append(docs, d.Path)
	}
	// Only tracked documents are context. The untracked CONTRIBUTING.md is a
	// new change (sent as a file like any untracked change), never context;
	// the ignored AGENTS.md is never sent at all.
	if strings.Join(docs, ",") != "README.md,docs/guide.md" {
		t.Fatalf("context documents = %v", docs)
	}
	if strings.Contains(ai.payload, "PRIVATE agent instructions") || strings.Contains(ai.payload, "AGENTS.md") {
		t.Fatal("ignored private document was sent")
	}
	if len(advice.Stage) != 2 || advice.Subject == "" {
		t.Fatalf("advice %+v", advice)
	}
	actions := map[string]string{}
	for _, c := range advice.Changes {
		actions[c.Path] = c.Action
	}
	if actions["a.txt"] != "stage" || actions["new.txt"] != "stage" || actions["b.txt"] != "unstage" {
		t.Fatalf("changes %+v", advice.Changes)
	}
	if err = a.ApplyStageAdvice(ctx, dir, advice); err != nil {
		t.Fatal(err)
	}
	staged, err := r.Exec(ctx, dir, "diff", "--cached", "--name-only")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(staged); strings.Join(got, ",") != "a.txt,new.txt" {
		t.Fatalf("stage must equal the advice, got %v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "b.txt"))
	if string(data) != "staged but unrelated\n" {
		t.Fatal("working file changed by unstaging")
	}
}

func TestStageAdviceRefusesChangesSinceAnalysisAndHoldsUnclassified(t *testing.T) {
	a, r, dir := stageFixture(t)
	ctx := context.Background()
	a.SetAIProvider(&stageAI{stage: []string{"a.txt"}}) // b.txt/new.txt omitted
	advice, err := a.StageAdviceAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(advice.Hold) != 3 { // b.txt, new.txt and the new CONTRIBUTING.md
		t.Fatalf("unclassified files must be held: %+v", advice.Hold)
	}
	before, _ := r.Exec(ctx, dir, "ls-files", "--stage")
	writeCommitFile(t, dir, "a.txt", "edited after the analysis\n")
	if err = a.ApplyStageAdvice(ctx, dir, advice); err == nil || !strings.Contains(err.Error(), "nothing applied") {
		t.Fatalf("stale advice must be refused: %v", err)
	}
	after, _ := r.Exec(ctx, dir, "ls-files", "--stage")
	if before != after {
		t.Fatal("index changed although the advice was refused")
	}
}
