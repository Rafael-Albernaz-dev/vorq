package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// reviewAI returns a review mixing valid and invalid findings.
type reviewAI struct{ aiFunc }

func (reviewAI) Analyze(_ context.Context, r domain.AnalysisRequest) (json.RawMessage, error) {
	var files []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(r.Payload), &files); err != nil {
		return nil, err
	}
	return json.Marshal(domain.DiffReview{Summary: "Overall fine", Findings: []domain.ReviewFinding{
		{FileID: files[0].ID, Severity: "error", Message: "Nil dereference", Evidence: "x.y()", Suggestion: "Check x before use"},
		{FileID: "F9999", Severity: "error", Message: "Unknown file", Evidence: "?"},
		{FileID: files[0].ID, Severity: "critical", Message: "Bad severity", Evidence: "x"},
		{FileID: files[0].ID, Severity: "warning", Message: "No evidence"},
	}})
}

// VORQ-014: one malformed finding no longer discards the whole review.
func TestReviewKeepsValidFindingsAndCountsDropped(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	a.SetAIProvider(reviewAI{})
	review, err := a.ReviewDiffAI(context.Background(), dir, "claude", "opus")
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Findings) != 1 || review.Findings[0].Suggestion != "Check x before use" || review.Dropped != 3 {
		t.Fatalf("findings=%+v dropped=%d", review.Findings, review.Dropped)
	}
}

func TestProviderAndModelValidationIsShared(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	ctx := context.Background()
	for _, c := range []struct {
		provider, model string
		ok              bool
	}{
		{"claude", "", true}, {"claude", "opus", true}, {"claude", "claude-sonnet-5-5", true},
		{"claude", "gpt-5", false}, {"claude", "--tools", false},
		{"codex", "gpt-5", true}, {"codex", "opus", false},
		{"gemini", "gemini-3", true}, {"ollama", "", false},
	} {
		_, err := a.ReviewDiffAI(ctx, dir, c.provider, c.model)
		invalid := err != nil && (strings.Contains(err.Error(), "invalid model") || strings.Contains(err.Error(), "choose codex"))
		if c.ok == invalid {
			t.Errorf("%s/%q: ok=%v but err=%v", c.provider, c.model, c.ok, err)
		}
	}
}

func TestSuggestionDiffLimitIsLarger(t *testing.T) {
	if app.MaxAIDiffBytes < 256*1024 || app.AnalysisLimit < 2*1024*1024 {
		t.Fatalf("limits regressed: diff=%d analysis=%d", app.MaxAIDiffBytes, app.AnalysisLimit)
	}
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "big.txt", strings.Repeat("line of text\n", 100*1024/13)) // ~100 KiB: accepted now
	if err := r.Add(ctx, dir, "big.txt"); err != nil {
		t.Fatal(err)
	}
	sent := 0
	a.SetAIProvider(aiFunc(func(_ context.Context, req domain.AIRequest) (*domain.CommitProposal, error) {
		sent = len(req.Diff)
		return &domain.CommitProposal{Subject: "feat: add big text", Body: "Add a large text file."}, nil
	}))
	if _, err := a.SuggestCommitAI(ctx, dir, "claude"); err != nil || sent < 100*1024 {
		t.Fatalf("100 KiB diff should be accepted untruncated: sent=%d err=%v", sent, err)
	}
	writeCommitFile(t, dir, "huge.txt", strings.Repeat("x", app.MaxAIDiffBytes+1))
	if err := r.Add(ctx, dir, "huge.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SuggestCommitAI(ctx, dir, "claude"); err == nil || !strings.Contains(err.Error(), "nothing sent") {
		t.Fatalf("above the limit must be refused before sending: %v", err)
	}
}

// pathPlanAI answers with file paths instead of IDs, as Claude did in QA.
type pathPlanAI struct{ aiFunc }

func (pathPlanAI) Analyze(_ context.Context, r domain.AnalysisRequest) (json.RawMessage, error) {
	var files []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(r.Payload), &files); err != nil {
		return nil, err
	}
	var groups []domain.PlannedCommit
	for _, f := range files {
		groups = append(groups, domain.PlannedCommit{Subject: "feat: update file", Body: "Update.", Reason: "Fixture", Files: []string{"./" + f.Path}})
	}
	return json.Marshal(map[string]any{"commits": groups})
}

func TestPlanAcceptsPathsForUniqueFiles(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	a.SetAIProvider(pathPlanAI{})
	plan, err := a.PlanCommitsAI(context.Background(), dir, "claude", "haiku")
	if err != nil {
		t.Fatalf("plan with paths must be resolved to IDs: %v", err)
	}
	for _, c := range plan.Commits {
		for _, id := range c.Files {
			if !strings.HasPrefix(id, "F") {
				t.Fatalf("file reference not normalised to an ID: %q", id)
			}
		}
	}
}
