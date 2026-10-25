package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type aiFunc func(context.Context, domain.AIRequest) (*domain.CommitProposal, error)

func (f aiFunc) Suggest(ctx context.Context, r domain.AIRequest) (*domain.CommitProposal, error) {
	return f(ctx, r)
}

func TestAISuggestionOnlyStagedAndRevalidates(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "staged.txt", "staged content\n")
	if err := r.Add(ctx, dir, "staged.txt"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "staged.txt", "working secret\n")
	writeCommitFile(t, dir, "untracked.txt", "untracked secret\n")
	calls := 0
	a.SetAIProvider(aiFunc(func(_ context.Context, req domain.AIRequest) (*domain.CommitProposal, error) {
		calls++
		if req.Provider != "codex" || !strings.Contains(req.Diff, "staged content") || strings.Contains(req.Diff, "secret") {
			t.Fatalf("wrong input: %+v", req)
		}
		if req.Model != "gpt-test" {
			t.Fatal("selected model not forwarded")
		}
		return &domain.CommitProposal{Subject: "feat: add staged content", Body: "Add the staged text file."}, nil
	}))
	p, err := a.SuggestCommitAI(ctx, dir, "codex", "gpt-test")
	if err != nil || calls != 1 || p.Proposal.Engine != "codex" {
		t.Fatalf("proposal: %+v %v", p, err)
	}
	if err = a.ValidateAISuggestion(ctx, dir, p.Diff); err != nil {
		t.Fatal(err)
	}
	if err = r.Add(ctx, dir, "staged.txt"); err != nil {
		t.Fatal(err)
	}
	if err = a.ValidateAISuggestion(ctx, dir, p.Diff); err == nil {
		t.Fatal("changed index accepted")
	}
	a.SetAIProvider(aiFunc(func(_ context.Context, _ domain.AIRequest) (*domain.CommitProposal, error) {
		writeCommitFile(t, dir, "staged.txt", "changed while generating\n")
		if err := r.Add(ctx, dir, "staged.txt"); err != nil {
			t.Fatal(err)
		}
		return &domain.CommitProposal{Subject: "fix: update text"}, nil
	}))
	if _, err = a.SuggestCommitAI(ctx, dir, "gemini"); err == nil || !strings.Contains(err.Error(), "stage changed") {
		t.Fatalf("stale result accepted: %v", err)
	}
}

func TestAISuggestionLimitsFailuresAndInvalidOutput(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	calls := 0
	a.SetAIProvider(aiFunc(func(context.Context, domain.AIRequest) (*domain.CommitProposal, error) {
		calls++
		return nil, errors.New("quota exceeded")
	}))
	if _, err := a.SuggestCommitAI(ctx, dir, "codex"); err == nil || calls != 0 {
		t.Fatal("empty stage sent")
	}
	writeCommitFile(t, dir, "large.txt", strings.Repeat("x", app.MaxAIDiffBytes+1))
	if err := r.Add(ctx, dir, "large.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SuggestCommitAI(ctx, dir, "codex"); err == nil || calls != 0 {
		t.Fatal("oversized stage sent")
	}
	writeCommitFile(t, dir, "large.txt", "small\n")
	if err := r.Add(ctx, dir, "large.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SuggestCommitAI(ctx, dir, "codex"); err == nil || calls != 1 {
		t.Fatal("failure or provider call lost")
	}
	for _, subject := range []string{"", "made a change", "fix: line\nbreak", "fix: \x1b[31m", strings.Repeat("x", 101)} {
		t.Run(subject, func(t *testing.T) {
			a.SetAIProvider(aiFunc(func(context.Context, domain.AIRequest) (*domain.CommitProposal, error) {
				return &domain.CommitProposal{Subject: subject}, nil
			}))
			if _, err := a.SuggestCommitAI(ctx, dir, "codex"); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := a.SuggestCommitAI(ctx, dir, "gemini"); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
