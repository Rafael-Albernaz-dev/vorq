package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/suggest"
)

// AI limits (VORQ-014): fixed, shared by CLI and TUI. Inputs are never
// truncated: anything above a limit is refused before sending.
const (
	MaxAIDiffBytes    = 256 * 1024 // staged diff for message suggestions
	AISuggestTimeout  = 3 * time.Minute
	AIModelsTimeout   = 15 * time.Second
	AnalysisTimeout   = 5 * time.Minute
	MaxReviewFindings = 100
)

// AIProviders are the supported subscription clients, in menu order.
var AIProviders = []string{"codex", "gemini", "claude"}

// ClaudeModelAliases always resolve to the latest model of each family.
var ClaudeModelAliases = []string{"fable", "opus", "sonnet", "haiku"}

// validateProvider is the single provider/model rule for every AI use case.
func validateProvider(provider, model string) error {
	known := false
	for _, p := range AIProviders {
		known = known || p == provider
	}
	if !known {
		return fmt.Errorf("choose codex, gemini or claude")
	}
	if model == "" {
		return nil
	}
	valid := aiModelPattern.MatchString(model)
	switch provider {
	case "codex":
		valid = valid && strings.HasPrefix(model, "gpt-")
	case "gemini":
		valid = valid && strings.HasPrefix(model, "gemini-")
	case "claude":
		alias := false
		for _, a := range ClaudeModelAliases {
			alias = alias || a == model
		}
		valid = valid && (alias || strings.HasPrefix(model, "claude-"))
	}
	if !valid {
		return fmt.Errorf("invalid model %q for %s", model, provider)
	}
	return nil
}

var aiSubjectPattern = regexp.MustCompile(`^(feat|fix|chore|refactor|docs|test|style|perf)(\([^():\r\n]+\))?!?: .+$`)
var aiModelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,99}$`)

// AIProvider is every capability the AI use cases need (VORQ-017).
type AIProvider interface {
	Suggest(context.Context, domain.AIRequest) (*domain.CommitProposal, error)
	Analyze(context.Context, domain.AnalysisRequest) (json.RawMessage, error)
	ListModels(context.Context, string) ([]domain.AIModel, error)
}

type AISuggestionService interface {
	ListAIModels(context.Context, string) ([]domain.AIModel, error)
	SuggestCommitAI(context.Context, string, string, ...string) (*domain.AIProposal, error)
	ValidateAISuggestion(context.Context, string, string) error
}

func (a *App) ListAIModels(ctx context.Context, provider string) ([]domain.AIModel, error) {
	if err := validateProvider(provider, ""); err != nil {
		return nil, err
	}
	if a.ai == nil {
		return nil, fmt.Errorf("AI provider unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, AIModelsTimeout)
	defer cancel()
	return a.ai.ListModels(ctx, provider)
}

func (a *App) SetAIProvider(provider AIProvider) { a.ai = provider }

// SuggestCommitAI never stages, writes a commit, or silently changes providers.
func (a *App) SuggestCommitAI(ctx context.Context, dir, provider string, models ...string) (*domain.AIProposal, error) {
	if a.ai == nil {
		return nil, fmt.Errorf("AI provider unavailable")
	}
	model := ""
	if len(models) > 0 {
		model = models[0]
	}
	if err := validateProvider(provider, model); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, AISuggestTimeout)
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
