package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// VORQ-016: AI-written PR title and description from the whole branch
// (commits and base...HEAD diff, locally known base, no fetch), the repo's PR
// template and tracked context documents. It only drafts: the reviewed form
// and the existing publish checks still decide what is sent to GitHub.

const MaxPRTitle = 256
const MaxPRBody = 60000

type PullRequestAIService interface {
	DraftPullRequestAI(context.Context, string, string, string, string) (*domain.PRDraft, error)
}

func (a *App) DraftPullRequestAI(ctx context.Context, dir, base, provider, model string) (*domain.PRDraft, error) {
	if err := validateProvider(provider, model); err != nil {
		return nil, err
	}
	if a.ai == nil {
		return nil, fmt.Errorf("analysis provider unavailable")
	}
	p := a.ai
	draft, err := a.PreparePullRequest(ctx, dir, base)
	if err != nil {
		return nil, err
	}
	r := a.gitRunner
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, AnalysisTimeout)
	defer cancel()
	link, err := a.GetRepositoryLink(ctx, proj.RootPath)
	if err != nil {
		return nil, err
	}
	changes, err := r.BranchChanges(ctx, proj.RootPath, link.Remote, draft.Base)
	if err != nil {
		return nil, err
	}
	docs, err := a.contextDocuments(ctx, proj.RootPath)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"head": draft.Head, "base": draft.Base, "base_ref": changes.BaseRef,
		"context_documents": docs, "commits": changes.Commits, "files": changes.Files,
	})
	if err != nil {
		return nil, err
	}
	if len(payload) > AnalysisLimit {
		return nil, fmt.Errorf("PR analysis exceeds %d MiB; write the description manually or split the branch (nothing sent)", AnalysisLimit/(1024*1024))
	}
	raw, err := p.Analyze(ctx, domain.AnalysisRequest{Provider: provider, Model: model, Task: "pr", Payload: string(payload)})
	if err != nil {
		return nil, err
	}
	var out struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("invalid PR draft JSON")
	}
	title, body := strings.TrimSpace(out.Title), strings.TrimSpace(out.Body)
	if err = validatePRText(title, body); err != nil {
		return nil, err
	}
	// The branch must not have moved while the provider was working.
	if oid, err := a.gitRunner.RevParseHead(ctx, proj.RootPath); err != nil || oid != draft.HeadOID {
		return nil, fmt.Errorf("HEAD changed during the analysis; generate the PR again")
	}
	draft.Title, draft.Body = title, body
	return draft, nil
}

func validatePRText(title, body string) error {
	if title == "" || utf8.RuneCountInString(title) > MaxPRTitle || strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("AI returned an invalid PR title")
	}
	if body == "" || len(body) > MaxPRBody {
		return fmt.Errorf("AI returned an invalid PR description length")
	}
	for _, s := range []string{title, body} {
		if !utf8.ValidString(s) || strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
			return fmt.Errorf("AI returned control characters in the PR text")
		}
	}
	return nil
}
