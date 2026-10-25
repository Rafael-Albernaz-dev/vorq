package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/suggest"
)

const MaxAIDiffBytes = 48 * 1024

var aiSubjectPattern = regexp.MustCompile(`^(feat|fix|chore|refactor|docs|test|style|perf)(\([^():\r\n]+\))?!?: .+$`)
var aiModelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,99}$`)

type AIProvider interface {
	Suggest(context.Context, domain.AIRequest) (*domain.CommitProposal, error)
}

type AISuggestionService interface {
	ListAIModels(context.Context, string) ([]domain.AIModel, error)
	SuggestCommitAI(context.Context, string, string, ...string) (*domain.AIProposal, error)
	ValidateAISuggestion(context.Context, string, string) error
}

func (a *App) ListAIModels(ctx context.Context, provider string) ([]domain.AIModel, error) {
	if provider != "codex" && provider != "gemini" {
		return nil, fmt.Errorf("choose codex or gemini")
	}
	lister, ok := a.ai.(interface {
		ListModels(context.Context, string) ([]domain.AIModel, error)
	})
	if !ok {
		return nil, fmt.Errorf("model catalog unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return lister.ListModels(ctx, provider)
}

func (a *App) SetAIProvider(provider AIProvider) { a.ai = provider }

// SuggestCommitAI never stages, writes a commit, or silently changes providers.
func (a *App) SuggestCommitAI(ctx context.Context, dir, provider string, models ...string) (*domain.AIProposal, error) {
	if provider != "codex" && provider != "gemini" {
		return nil, fmt.Errorf("choose codex or gemini")
	}
	if a.ai == nil {
		return nil, fmt.Errorf("AI provider unavailable")
	}
	model := ""
	if len(models) > 0 {
		model = models[0]
	}
	if model != "" && (!aiModelPattern.MatchString(model) || provider == "gemini" && !strings.HasPrefix(model, "gemini-") || provider == "codex" && !strings.HasPrefix(model, "gpt-")) {
		return nil, fmt.Errorf("invalid model for %s", provider)
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cs, err := a.GetCommitChanges(ctx, dir)
	if err != nil {
		return nil, err
	}
	if cs.StagedCount() == 0 || strings.TrimSpace(cs.StagedDiff) == "" {
		return nil, fmt.Errorf("no staged diff to analyze")
	}
	if len(cs.StagedDiff) > MaxAIDiffBytes {
		return nil, fmt.Errorf("staged diff exceeds %d KiB; split the commit or use local rules (nothing sent)", MaxAIDiffBytes/1024)
	}
	p, err := a.ai.Suggest(ctx, domain.AIRequest{Provider: provider, Model: model, Diff: cs.StagedDiff})
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = validateAIMessage(p); err != nil {
		return nil, err
	}
	if err = a.ValidateAISuggestion(ctx, cs.WorkspacePath, cs.StagedDiff); err != nil {
		return nil, err
	}
	p.Engine = domain.SuggestionEngineType(provider)
	p.GeneratedAt = time.Now().UTC()
	return &domain.AIProposal{Proposal: *p, Diff: cs.StagedDiff}, nil
}

func (a *App) ValidateAISuggestion(ctx context.Context, dir, diff string) error {
	cs, err := a.GetCommitChanges(ctx, dir)
	if err != nil {
		return err
	}
	if cs.StagedDiff != diff {
		return fmt.Errorf("stage changed; generate a new suggestion")
	}
	return nil
}

func validateAIMessage(p *domain.CommitProposal) error {
	if p == nil {
		return fmt.Errorf("AI returned no proposal")
	}
	p.Subject, p.Body = strings.TrimSpace(p.Subject), strings.TrimSpace(p.Body)
	if len([]rune(p.Subject)) == 0 || len([]rune(p.Subject)) > 100 || len(p.Body) > 8192 {
		return fmt.Errorf("AI returned an invalid message length")
	}
	for _, s := range []string{p.Subject, p.Body} {
		if !utf8.ValidString(s) {
			return fmt.Errorf("AI returned invalid UTF-8")
		}
		if strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) >= 0 {
			return fmt.Errorf("AI returned control characters")
		}
	}
	if strings.ContainsAny(p.Subject, "\n\t") {
		return fmt.Errorf("AI returned a multiline subject")
	}
	header, subject, ok := strings.Cut(p.Subject, ": ")
	typ := strings.SplitN(strings.TrimSuffix(header, "!"), "(", 2)[0]
	if !ok || strings.TrimSpace(subject) == "" || !suggest.ValidType(typ) || !aiSubjectPattern.MatchString(p.Subject) {
		return fmt.Errorf("AI returned an invalid Conventional Commit subject")
	}
	return nil
}
