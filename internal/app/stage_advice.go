package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// VORQ-015: AI advice on what belongs in the next commit, using the eligible
// working changes (staged or not) plus the repository's own tracked context
// documents. Applying it makes the index match the advice exactly; working
// files are never modified and nothing is committed or pushed.

const (
	MaxContextDocument = 32 * 1024 // per document, truncation is marked
	MaxContextTotal    = 96 * 1024
)

type StageAdviceService interface {
	StageAdviceAI(context.Context, string, string, string) (*domain.StageAdvice, error)
	ApplyStageAdvice(context.Context, string, *domain.StageAdvice) error
}

// isContextDocument is the policy for documents that describe how the
// project expects changes to be made. Only tracked files are considered.
func isContextDocument(p string) bool {
	lower := strings.ToLower(p)
	base := path.Base(lower)
	dir := path.Dir(lower)
	switch {
	case dir == "." && (strings.HasPrefix(base, "readme") || strings.HasPrefix(base, "contributing")):
		return true
	case dir == "." && (base == ".gitmessage" || strings.HasPrefix(base, "commitlint.config") || strings.HasPrefix(base, ".commitlintrc")):
		return true
	case dir == ".github" && (strings.HasSuffix(base, ".md") || strings.HasPrefix(base, "pull_request_template")):
		return true
	case dir == "docs" && strings.HasSuffix(base, ".md"):
		return true
	}
	return false
}

// contextDocuments reads tracked context documents from the working tree,
// within fixed limits; truncation is written into the content explicitly.
func (a *App) contextDocuments(ctx context.Context, root string) ([]domain.ContextDocument, error) {
	tracked, err := a.gitRunner.IndexedPaths(ctx, root)
	if err != nil {
		return nil, err
	}
	sort.Strings(tracked)
	fs, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	scanner, err := guardScanner(root)
	if err != nil {
		return nil, err
	}
	docs := []domain.ContextDocument{}
	total := 0
	for _, p := range tracked {
		if !isContextDocument(p) || validateViewerPath(p) != nil {
			continue
		}
		info, err := fs.Lstat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue // deleted or replaced in the working tree
		}
		budget := min(MaxContextDocument, MaxContextTotal-total)
		if budget <= 0 {
			break
		}
		f, err := fs.Open(p)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, int64(budget)+1))
		f.Close()
		if err != nil {
			return nil, err
		}
		doc := domain.ContextDocument{Path: p}
		if len(data) > budget {
			data, doc.Truncated = data[:budget], true
		}
		for !utf8.Valid(data) && len(data) > 0 {
			data = data[:len(data)-1]
		}
		doc.Content = string(data)
		if withheldFromAI(scanner, p, doc.Content) {
			continue // VORQ-018: a context document with a possible secret is not sent
		}
		if doc.Truncated {
			doc.Content += fmt.Sprintf("\n[truncated by vorq: document larger than %d bytes]", budget)
		}
		total += len(data)
		docs = append(docs, doc)
	}
	return docs, nil
}

func (a *App) StageAdviceAI(ctx context.Context, dir, provider, model string) (*domain.StageAdvice, error) {
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return nil, err
	}
	docs, err := a.contextDocuments(ctx, proj.RootPath)
	if err != nil {
		return nil, err
	}
	snapshot, data, err := a.analysisWithContext(ctx, proj.RootPath, provider, model, "stage", docs)
	if err != nil {
		return nil, err
	}
	var response struct {
		Summary string                   `json:"summary"`
		Stage   []domain.StageAdviceItem `json:"stage"`
		Hold    []domain.StageAdviceItem `json:"hold"`
		Subject string                   `json:"subject"`
		Body    string                   `json:"body"`
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid stage advice JSON")
	}
	advice := &domain.StageAdvice{Summary: response.Summary, Subject: response.Subject, Body: response.Body, Snapshot: *snapshot}
	for _, d := range docs {
		advice.Context = append(advice.Context, d.Path)
	}
	if len(advice.Summary) > 16384 {
		return nil, fmt.Errorf("stage advice summary exceeds limits")
	}
	// Each eligible file ends in exactly one list. Unknown or repeated
	// references are dropped and counted; unclassified files are held.
	resolve := fileResolver(snapshot)
	seen := map[string]bool{}
	eligible := map[string]bool{}
	for _, f := range snapshot.Files {
		if f.Excluded == "" {
			eligible[f.ID] = true
		}
	}
	classify := func(items []domain.StageAdviceItem) []domain.StageAdviceItem {
		var kept []domain.StageAdviceItem
		for _, item := range items {
			item.FileID = resolve(item.FileID)
			if !eligible[item.FileID] || seen[item.FileID] || len(item.Reason) > 8192 || len(item.Risk) > 8192 {
				advice.Dropped++
				continue
			}
			seen[item.FileID] = true
			kept = append(kept, item)
		}
		return kept
	}
	advice.Stage = classify(response.Stage)
	advice.Hold = classify(response.Hold)
	for _, f := range snapshot.Files {
		if eligible[f.ID] && !seen[f.ID] {
			advice.Hold = append(advice.Hold, domain.StageAdviceItem{FileID: f.ID, Reason: "not classified by the AI; kept out of the stage"})
		}
	}
	proposal := &domain.CommitProposal{Subject: advice.Subject, Body: advice.Body}
	if len(advice.Stage) == 0 || validateAIMessage(proposal) != nil {
		advice.Subject, advice.Body = "", "" // no usable message for this advice
	} else {
		advice.Subject, advice.Body = proposal.Subject, proposal.Body
	}
	if advice.Changes, err = a.stageChanges(ctx, proj.RootPath, advice); err != nil {
		return nil, err
	}
	return advice, nil
}

// stageChanges lists the index updates needed so the stage equals the
// advice, skipping files already in the desired state.
func (a *App) stageChanges(ctx context.Context, root string, advice *domain.StageAdvice) ([]domain.StageChange, error) {
	cs, err := a.gitRunner.ReadChanges(ctx, root, "")
	if err != nil {
		return nil, err
	}
	status := map[string]domain.FileStatus{}
	for _, f := range cs.Files {
		status[f.Path] = f
	}
	byID := map[string]domain.SnapshotFile{}
	for _, f := range advice.Snapshot.Files {
		byID[f.ID] = f
	}
	var changes []domain.StageChange
	add := func(items []domain.StageAdviceItem, action string) {
		for _, item := range items {
			f := byID[item.FileID]
			s := status[f.Path]
			partial := s.IsStaged() && s.IsUnstaged()
			if action == "stage" && s.IsStaged() && !s.IsUnstaged() && !s.IsUntracked() {
				continue // already exactly staged
			}
			if action == "unstage" && !s.IsStaged() {
				continue // nothing staged for this file
			}
			changes = append(changes, domain.StageChange{Path: f.Path, OldPath: f.OldPath, Action: action, Partial: partial})
		}
	}
	add(advice.Stage, "stage")
	add(advice.Hold, "unstage")
	return changes, nil
}

// ApplyStageAdvice revalidates the analysed snapshot (HEAD, index and every
// working file) and then updates only the index. Excluded files are untouched.
func (a *App) ApplyStageAdvice(ctx context.Context, dir string, advice *domain.StageAdvice) error {
	if advice == nil || len(advice.Stage) == 0 {
		return fmt.Errorf("the advice recommends nothing to stage")
	}
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return err
	}
	if proj.RootPath != advice.Snapshot.Workspace {
		return fmt.Errorf("advice belongs to another workspace")
	}
	r := a.gitRunner
	if err = r.ValidateSnapshot(ctx, advice.Snapshot, advice.Snapshot.Head, advice.Snapshot.Index); err != nil {
		return fmt.Errorf("changes since the analysis; run it again (nothing applied): %w", err)
	}
	excluded := map[string]bool{}
	for _, f := range advice.Snapshot.Files {
		if f.Excluded != "" {
			excluded[f.Path] = true
		}
	}
	var stage, unstage []string
	for _, c := range advice.Changes {
		if c.Action == "stage" && excluded[c.Path] {
			return fmt.Errorf("%q was excluded from the analysis; it is never staged by the advice", c.Path)
		}
		paths := []string{"./" + c.Path}
		if c.OldPath != "" && c.OldPath != c.Path {
			paths = append(paths, "./"+c.OldPath)
		}
		if err = validateCommitPaths(paths); err != nil {
			return err
		}
		if c.Action == "stage" {
			stage = append(stage, paths...)
		} else {
			unstage = append(unstage, paths...)
		}
	}
	if len(unstage) > 0 {
		if err = a.UnstageFiles(ctx, proj.RootPath, unstage); err != nil {
			return err
		}
	}
	if len(stage) > 0 {
		if err = a.StageFiles(ctx, proj.RootPath, stage); err != nil {
			return fmt.Errorf("stage partially applied (files intact); inspect the stage: %w", err)
		}
	}
	return nil
}
