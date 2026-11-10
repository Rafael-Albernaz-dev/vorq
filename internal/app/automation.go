package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// AnalysisLimit bounds the JSON payload sent for review/plan (VORQ-014).
const AnalysisLimit = 2 * 1024 * 1024

type AutomationRepository interface {
	CaptureSnapshot(context.Context, string) (*domain.ChangeSnapshot, error)
	ValidateSnapshot(context.Context, domain.ChangeSnapshot, string, string) error
	CommitSnapshot(context.Context, domain.ChangeSnapshot, domain.PlannedCommit, string, string) (string, string, error)
}
type AutomationService interface {
	EditCommitPlan(context.Context, string, *domain.CommitPlan, domain.PlanEdit) (*domain.CommitPlan, error)
	ReviewDiffAI(context.Context, string, string, string) (*domain.DiffReview, error)
	PlanCommitsAI(context.Context, string, string, string) (*domain.CommitPlan, error)
	ValidateCommitPlan(context.Context, string, *domain.CommitPlan) error
	SaveCommitPlan(context.Context, string, *domain.CommitPlan) error
	LoadCommitPlan(context.Context, string) (*domain.CommitPlan, error)
	ExecuteCommitPlan(context.Context, string, *domain.CommitPlan) (*domain.CommitPlan, error)
}

func sha(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func (a *App) analysis(ctx context.Context, dir, provider, model, task string) (*domain.ChangeSnapshot, json.RawMessage, error) {
	return a.analysisWithContext(ctx, dir, provider, model, task, nil)
}

// analysisWithContext sends the eligible snapshot and, when given, tracked
// repository documents as {"context_documents":[...],"files":[...]}.
func (a *App) analysisWithContext(ctx context.Context, dir, provider, model, task string, docs []domain.ContextDocument) (*domain.ChangeSnapshot, json.RawMessage, error) {
	if err := validateProvider(provider, model); err != nil {
		return nil, nil, err
	}
	r := a.gitRunner
	if a.ai == nil {
		return nil, nil, fmt.Errorf("analysis provider unavailable")
	}
	p := a.ai
	ctx, cancel := context.WithTimeout(ctx, AnalysisTimeout)
	defer cancel()
	snapshot, err := r.CaptureSnapshot(ctx, dir)
	if err != nil {
		return nil, nil, err
	}
	var payload []map[string]string
	for _, f := range snapshot.Files {
		if f.Excluded == "" {
			payload = append(payload, map[string]string{"id": f.ID, "path": f.Path, "old_path": f.OldPath, "diff": f.Diff})
		}
	}
	if len(payload) == 0 {
		return nil, nil, fmt.Errorf("no eligible changes; ignored/binary/special/large files require manual handling")
	}
	var data []byte
	if docs != nil {
		data, err = json.Marshal(map[string]any{"context_documents": docs, "files": payload})
	} else {
		data, err = json.Marshal(payload)
	}
	if err != nil {
		return nil, nil, err
	}
	if len(data) > AnalysisLimit {
		return nil, nil, fmt.Errorf("analysis exceeds %d MiB; select a smaller change set (nothing sent)", AnalysisLimit/(1024*1024))
	}
	result, err := p.Analyze(ctx, domain.AnalysisRequest{Provider: provider, Model: model, Task: task, Payload: string(data)})
	if err != nil {
		return nil, nil, err
	}
	if err = r.ValidateSnapshot(ctx, *snapshot, snapshot.Head, snapshot.Index); err != nil {
		return nil, nil, err
	}
	return snapshot, result, nil
}

// fileResolver maps a provider's file reference to a snapshot ID. Models are
// asked for IDs but sometimes answer with the path; a path naming exactly one
// eligible file is accepted as that file's ID. Anything else is returned
// unchanged and later rejected by validation.
func fileResolver(s *domain.ChangeSnapshot) func(string) string {
	byPath := map[string]string{}
	ambiguous := map[string]bool{}
	ids := map[string]bool{}
	for _, f := range s.Files {
		if f.Excluded != "" {
			continue
		}
		ids[f.ID] = true
		for _, p := range []string{f.Path, f.OldPath} {
			if p == "" {
				continue
			}
			if other, ok := byPath[p]; ok && other != f.ID {
				ambiguous[p] = true
			}
			byPath[p] = f.ID
		}
	}
	return func(ref string) string {
		ref = strings.TrimSpace(ref)
		if ids[ref] {
			return ref
		}
		clean := strings.TrimPrefix(ref, "./")
		if id, ok := byPath[clean]; ok && !ambiguous[clean] {
			return id
		}
		return ref
	}
}

func (a *App) EditCommitPlan(ctx context.Context, dir string, original *domain.CommitPlan, edit domain.PlanEdit) (*domain.CommitPlan, error) {
	data, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var p domain.CommitPlan
	if err = json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if len(p.Completed) > 0 {
		return nil, fmt.Errorf("execution has started; resume it or generate a new plan for remaining changes")
	}
	if edit.Group < 0 || edit.Group >= len(p.Commits) {
		return nil, fmt.Errorf("invalid group")
	}
	switch edit.Kind {
	case "message":
		p.Commits[edit.Group].Subject = edit.Subject
		p.Commits[edit.Group].Body = edit.Body
	case "order":
		if edit.Target < 0 || edit.Target >= len(p.Commits) {
			return nil, fmt.Errorf("invalid target group")
		}
		g := p.Commits[edit.Group]
		p.Commits = append(p.Commits[:edit.Group], p.Commits[edit.Group+1:]...)
		p.Commits = append(p.Commits, domain.PlannedCommit{})
		copy(p.Commits[edit.Target+1:], p.Commits[edit.Target:])
		p.Commits[edit.Target] = g
	case "move", "exclude":
		if edit.Kind == "move" && (edit.Target < 0 || edit.Target >= len(p.Commits) || edit.Target == edit.Group) {
			return nil, fmt.Errorf("choose another valid group")
		}
		found := false
		var files []string
		for _, id := range p.Commits[edit.Group].Files {
			if id == edit.FileID {
				found = true
			} else {
				files = append(files, id)
			}
		}
		if !found {
			return nil, fmt.Errorf("file is not in the selected group")
		}
		p.Commits[edit.Group].Files = files
		if edit.Kind == "move" {
			p.Commits[edit.Target].Files = append(p.Commits[edit.Target].Files, edit.FileID)
		} else {
			for i := range p.Snapshot.Files {
				if p.Snapshot.Files[i].ID == edit.FileID {
					p.Snapshot.Files[i].Excluded = "excluded explicitly by user"
				}
			}
		}
		if len(files) == 0 {
			p.Commits = append(p.Commits[:edit.Group], p.Commits[edit.Group+1:]...)
		}
	default:
		return nil, fmt.Errorf("unknown plan edit")
	}
	if err = a.SaveCommitPlan(ctx, dir, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
func (a *App) ReviewDiffAI(ctx context.Context, dir, provider, model string) (*domain.DiffReview, error) {
	snapshot, data, err := a.analysis(ctx, dir, provider, model, "review")
	if err != nil {
		return nil, err
	}
	var review domain.DiffReview
	if err = json.Unmarshal(data, &review); err != nil {
		return nil, fmt.Errorf("invalid review JSON")
	}
	known := map[string]bool{}
	for _, f := range snapshot.Files {
		if f.Excluded == "" {
			known[f.ID] = true
		}
	}
	if len(review.Summary) > 16384 {
		return nil, fmt.Errorf("review summary exceeds limits")
	}
	// Keep every grounded finding; drop (and count) invalid ones instead of
	// discarding the whole review because of a single malformed entry.
	var kept []domain.ReviewFinding
	resolve := fileResolver(snapshot)
	for _, f := range review.Findings {
		f.FileID = resolve(f.FileID)
		valid := known[f.FileID] && strings.TrimSpace(f.Evidence) != "" && strings.TrimSpace(f.Message) != "" &&
			len(f.Message) <= 8192 && len(f.Evidence) <= 8192 && len(f.Suggestion) <= 8192 &&
			(f.Severity == "info" || f.Severity == "warning" || f.Severity == "error")
		if valid && len(kept) < MaxReviewFindings {
			kept = append(kept, f)
		} else {
			review.Dropped++
		}
	}
	review.Findings = kept
	review.Snapshot = *snapshot
	return &review, nil
}
func (a *App) PlanCommitsAI(ctx context.Context, dir, provider, model string) (*domain.CommitPlan, error) {
	snapshot, data, err := a.analysis(ctx, dir, provider, model, "plan")
	if err != nil {
		return nil, err
	}
	var response struct {
		Commits []domain.PlannedCommit `json:"commits"`
	}
	if err = json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid plan JSON")
	}
	resolve := fileResolver(snapshot)
	for i := range response.Commits {
		for j, ref := range response.Commits[i].Files {
			response.Commits[i].Files[j] = resolve(ref)
		}
	}
	plan := &domain.CommitPlan{Snapshot: *snapshot, Commits: response.Commits, ExpectedHead: snapshot.Head, ExpectedIndex: snapshot.Index}
	if err = a.ValidateCommitPlan(ctx, dir, plan); err != nil {
		return nil, err
	}
	if err = a.SaveCommitPlan(ctx, dir, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func (a *App) ValidateCommitPlan(ctx context.Context, dir string, p *domain.CommitPlan) error {
	if p == nil || len(p.Commits) == 0 || len(p.Commits) > 50 {
		return fmt.Errorf("invalid commit plan")
	}
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return err
	}
	if proj.RootPath != p.Snapshot.Workspace {
		return fmt.Errorf("plan belongs to another workspace")
	}
	known := map[string]bool{}
	paths := map[string]bool{}
	ids := map[string]bool{}
	oldPaths := map[string]bool{}
	for _, f := range p.Snapshot.Files {
		if err = validateViewerPath(f.Path); err != nil {
			return err
		}
		if f.OldPath != "" {
			if err = validateViewerPath(f.OldPath); err != nil {
				return err
			}
		}
		if ids[f.ID] || paths[f.Path] || f.ID == "" {
			return fmt.Errorf("duplicate snapshot entry")
		}
		paths[f.Path] = true
		ids[f.ID] = true
		if f.OldPath != "" && f.OldPath != f.Path {
			oldPaths[f.OldPath] = true
		}
		if f.Excluded == "" {
			known[f.ID] = true
			if f.Mode != "0" && f.Mode != "100644" && f.Mode != "100755" {
				return fmt.Errorf("unsupported file mode")
			}
			if f.Mode != "0" && sha(append([]byte(f.Mode+":"), f.Content...)) != f.Fingerprint {
				return fmt.Errorf("snapshot content changed")
			}
		}
	}
	for path := range oldPaths {
		if paths[path] {
			return fmt.Errorf("rename source reused by another change; handle manually")
		}
	}
	seen := map[string]bool{}
	for _, c := range p.Commits {
		proposal := &domain.CommitProposal{Subject: c.Subject, Body: c.Body}
		if err = validateAIMessage(proposal); err != nil {
			return err
		}
		if len(c.Files) == 0 || len(c.Reason) > 8192 {
			return fmt.Errorf("empty or invalid group")
		}
		for _, id := range c.Files {
			if !known[id] || seen[id] {
				return fmt.Errorf("unknown, excluded or repeated file %q", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != len(known) {
		return fmt.Errorf("plan omits eligible changes")
	}
	if len(p.Completed) > len(p.Commits) {
		return fmt.Errorf("invalid execution progress")
	}
	if len(p.Completed) == 0 && p.ExpectedHead != p.Snapshot.Head {
		return fmt.Errorf("invalid starting HEAD")
	}
	if len(p.Completed) > 0 && p.ExpectedHead != p.Completed[len(p.Completed)-1].Hash {
		return fmt.Errorf("invalid progress HEAD")
	}
	return nil
}

func planDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "vorq", "plans"), nil
}
func (a *App) SaveCommitPlan(ctx context.Context, dir string, p *domain.CommitPlan) error {
	if err := a.ValidateCommitPlan(ctx, dir, p); err != nil {
		return err
	}
	data, _ := json.Marshal(p.Commits)
	p.ID = sha(append([]byte(p.Snapshot.ID), data...))
	base, err := planDirectory()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		return err
	}
	data, err = json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(base, ".plan-")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(base, p.ID+".json")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, sha([]byte(p.Snapshot.Workspace))+".last"), []byte(p.ID), 0600)
}
func (a *App) LoadCommitPlan(ctx context.Context, dir string) (*domain.CommitPlan, error) {
	proj, err := a.ResolveProject(ctx, dir)
	if err != nil {
		return nil, err
	}
	base, err := planDirectory()
	if err != nil {
		return nil, err
	}
	id, err := os.ReadFile(filepath.Join(base, sha([]byte(proj.RootPath))+".last"))
	if err != nil {
		return nil, fmt.Errorf("no saved plan for this workspace")
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).Match(id) {
		return nil, fmt.Errorf("invalid saved plan ID")
	}
	data, err := os.ReadFile(filepath.Join(base, string(id)+".json"))
	if err != nil {
		return nil, err
	}
	var p domain.CommitPlan
	if err = json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if err = a.ValidateCommitPlan(ctx, dir, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
func (a *App) ExecuteCommitPlan(ctx context.Context, dir string, reviewed *domain.CommitPlan) (*domain.CommitPlan, error) {
	data, err := json.Marshal(reviewed)
	if err != nil {
		return nil, err
	}
	var p domain.CommitPlan
	if err = json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if err = a.ValidateCommitPlan(ctx, dir, &p); err != nil {
		return &p, err
	}
	if strings.HasPrefix(p.Error, "INSPECT:") {
		return &p, fmt.Errorf("inspect the previously created commit/index before making a new plan")
	}
	r := a.gitRunner
	p.Error = ""
	for i := len(p.Completed); i < len(p.Commits); i++ {
		if err = r.ValidateSnapshot(ctx, p.Snapshot, p.ExpectedHead, p.ExpectedIndex); err != nil {
			p.Error = err.Error()
			a.SaveCommitPlan(context.WithoutCancel(ctx), dir, &p)
			return &p, err
		}
		stepCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		hash, index, e := r.CommitSnapshot(stepCtx, p.Snapshot, p.Commits[i], p.ExpectedHead, p.ExpectedIndex)
		cancel()
		if hash != "" {
			p.Completed = append(p.Completed, domain.PlanStep{Hash: hash, Subject: p.Commits[i].Subject})
			p.ExpectedHead = hash
			if index != "" {
				p.ExpectedIndex = index
			}
		}
		if e != nil {
			p.Error = e.Error()
			var metadataErr *domain.CommitMetadataError
			if hash != "" || errors.As(e, &metadataErr) {
				p.Error = "INSPECT: " + p.Error
			}
			a.SaveCommitPlan(context.WithoutCancel(ctx), dir, &p)
			return &p, e
		}
		if hash == "" {
			return &p, fmt.Errorf("native commit returned no hash")
		}
		if e = a.SaveCommitPlan(context.WithoutCancel(ctx), dir, &p); e != nil {
			p.Error = "INSPECT: commit exists, but progress could not be saved"
			return &p, e
		}
	}
	return &p, nil
}
