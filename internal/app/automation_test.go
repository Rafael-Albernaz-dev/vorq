package app_test

import (
	"context"
	"encoding/json"
	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type planningAI struct {
	aiFunc
	omit bool
	seen string
}

func (p *planningAI) Analyze(_ context.Context, r domain.AnalysisRequest) (json.RawMessage, error) {
	p.seen = r.Payload
	var files []struct {
		ID   string `json:"id"`
		Path string `json:"path"`
		Diff string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(r.Payload), &files); err != nil {
		return nil, err
	}
	if r.Task == "review" {
		return json.Marshal(map[string]any{"summary": "Review of supplied changes", "findings": []domain.ReviewFinding{}})
	}
	var groups []domain.PlannedCommit
	for _, f := range files {
		groups = append(groups, domain.PlannedCommit{Subject: "feat: update " + filepath.Base(f.Path), Body: "Update the reviewed working version.", Reason: "Independent fixture change", Files: []string{f.ID}})
	}
	if p.omit && len(groups) > 0 {
		groups = groups[:len(groups)-1]
	}
	return json.Marshal(map[string]any{"commits": groups})
}
func prepareAutomation(t *testing.T) (*app.App, string, *planningAI) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		writeCommitFile(t, dir, name, "base\n")
	}
	if err := r.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(ctx, dir, "test: baseline"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		writeCommitFile(t, dir, name, "staged "+name+"\n")
	}
	if err := r.Add(ctx, dir, "a.txt", "b.txt", "c.txt"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "a.txt", "working a.txt\n")
	writeCommitFile(t, dir, "new ação [x].txt", "new working\n")
	p := &planningAI{}
	a.SetAIProvider(p)
	return a, dir, p
}
func groupForPath(p *domain.CommitPlan, path string) int {
	for _, f := range p.Snapshot.Files {
		if f.Path == path {
			for i, g := range p.Commits {
				for _, id := range g.Files {
					if id == f.ID {
						return i
					}
				}
			}
		}
	}
	return -1
}

func TestAutomationPinnedGroupsPreserveOtherStage(t *testing.T) {
	a, dir, ai := prepareAutomation(t)
	ctx := context.Background()
	before, err := a.GetCommitChanges(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.PlanCommitsAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := a.GetCommitChanges(ctx, dir)
	if before.StagedDiff != after.StagedDiff {
		t.Fatal("planning changed stage")
	}
	if !strings.Contains(ai.seen, "working a.txt") || !strings.Contains(ai.seen, "new working") {
		t.Fatal("working/untracked changes omitted")
	}
	i := groupForPath(p, "c.txt")
	p, err = a.EditCommitPlan(ctx, dir, p, domain.PlanEdit{Kind: "exclude", Group: i, FileID: p.Commits[i].Files[0]})
	if err != nil {
		t.Fatal(err)
	}
	p, err = a.ExecuteCommitPlan(ctx, dir, p)
	if err != nil {
		t.Fatalf("execution: %v %+v", err, p)
	}
	if len(p.Completed) != 3 {
		t.Fatalf("wrong progress %+v", p.Completed)
	}
	cs, err := a.GetCommitChanges(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cs.StagedCount() != 1 || !strings.Contains(cs.StagedDiff, "c.txt") {
		t.Fatalf("other stage lost: %+v", cs)
	}
	if strings.Contains(cs.StagedDiff, "a.txt") || strings.Contains(cs.StagedDiff, "new ação") {
		t.Fatal("committed groups remain staged")
	}
	loaded, err := a.LoadCommitPlan(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.ExecuteCommitPlan(ctx, dir, loaded)
	if err != nil || len(again.Completed) != 3 {
		t.Fatalf("repeat duplicated/failed: %v", err)
	}
}
func TestAutomationStopsAndResumesWithoutDuplicating(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	ctx := context.Background()
	p, err := a.PlanCommitsAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	// Hook runs with the private index, so only the planned group's files appear.
	hookDir := filepath.Join(t.TempDir(), "hooks")
	os.MkdirAll(hookDir, 0700)
	script := "#!/bin/sh\nif git diff --cached --name-only | grep -qx b.txt; then exit 1; fi\n"
	if err = os.WriteFile(filepath.Join(hookDir, "pre-commit"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	r := newTestRunner(t)
	if _, err = r.Exec(ctx, dir, "config", "core.hooksPath", hookDir); err != nil {
		t.Fatal(err)
	}
	p, err = a.ExecuteCommitPlan(ctx, dir, p)
	if err == nil || len(p.Completed) != 1 {
		t.Fatalf("partial result lost: %v %+v", err, p.Completed)
	}
	first := p.Completed[0].Hash
	if err = os.WriteFile(filepath.Join(hookDir, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p, err = a.LoadCommitPlan(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err = a.ExecuteCommitPlan(ctx, dir, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Completed) != 4 || p.Completed[0].Hash != first {
		t.Fatal("resume duplicated first step")
	}
}
func TestAutomationRejectsStaleAndIncompletePlans(t *testing.T) {
	for _, kind := range []string{"file", "stage", "head", "omit", "lock"} {
		t.Run(kind, func(t *testing.T) {
			a, dir, ai := prepareAutomation(t)
			ctx := context.Background()
			if kind == "omit" {
				ai.omit = true
				if _, err := a.PlanCommitsAI(ctx, dir, "codex", ""); err == nil {
					t.Fatal("incomplete plan accepted")
				}
				return
			}
			p, err := a.PlanCommitsAI(ctx, dir, "codex", "")
			if err != nil {
				t.Fatal(err)
			}
			r := newTestRunner(t)
			switch kind {
			case "file":
				writeCommitFile(t, dir, "a.txt", "changed after review\n")
			case "stage":
				if err = r.Add(ctx, dir, "a.txt"); err != nil {
					t.Fatal(err)
				}
			case "head":
				if _, err = r.Exec(ctx, dir, "commit", "--allow-empty", "--only", "-m", "test: concurrent"); err != nil {
					t.Fatal(err)
				}
			case "lock":
				if err = os.WriteFile(filepath.Join(dir, ".git", "index.lock"), []byte("human lock"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := a.ExecuteCommitPlan(ctx, dir, p)
			if err == nil || len(result.Completed) != 0 {
				t.Fatalf("stale plan executed: %v", err)
			}
			if kind == "lock" {
				data, _ := os.ReadFile(filepath.Join(dir, ".git", "index.lock"))
				if string(data) != "human lock" {
					t.Fatal("deleted foreign lock")
				}
			}
		})
	}
}
func TestAutomationAcceptsMoreThanLegacy48KiB(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	writeCommitFile(t, dir, "large-review.txt", strings.Repeat("readable line\n", 5000))
	review, err := a.ReviewDiffAI(context.Background(), dir, "codex", "")
	if err != nil || review == nil {
		t.Fatalf("legacy limit retained: %v", err)
	}
}

func newTestRunner(t *testing.T) *git.NativeRunner {
	t.Helper()
	r, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAutomationRenameDeletionExclusionsAndFileMode(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	ctx := context.Background()
	r := newTestRunner(t)
	// A staged rename must include its source deletion in the approved diff.
	writeCommitFile(t, dir, "b.txt", "base\n")
	if err := r.Add(ctx, dir, "b.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exec(ctx, dir, "mv", "b.txt", "renamed ação.txt"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, ".gitignore", "private/\n")
	writeCommitFile(t, dir, "private/secret.txt", "NEVER_SEND\n")
	writeCommitFile(t, dir, "binary.dat", "binary\x00bytes")
	writeCommitFile(t, dir, "huge.txt", strings.Repeat("x", git.MaxAnalysisFile+1))
	if err := os.Symlink("private/secret.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exec(ctx, dir, "config", "core.filemode", "false"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "a.txt"), 0755); err != nil {
		t.Fatal(err)
	}
	p, err := a.PlanCommitsAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	excluded := map[string]bool{}
	for _, f := range p.Snapshot.Files {
		if f.Excluded != "" {
			excluded[f.Path] = true
		}
		if f.Path == "renamed ação.txt" && (!strings.Contains(f.Diff, "b.txt") || f.OldPath != "b.txt") {
			t.Fatalf("rename preview incomplete: %+v", f)
		}
		if f.Path == "a.txt" && f.Mode != "100644" {
			t.Fatal("core.filemode ignored")
		}
		if strings.HasPrefix(f.Path, "private/") {
			t.Fatal("ignored path in snapshot")
		}
	}
	for _, path := range []string{"binary.dat", "huge.txt", "link"} {
		if !excluded[path] {
			t.Fatalf("not excluded: %s", path)
		}
	}
	p, err = a.ExecuteCommitPlan(ctx, dir, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Completed) != 5 {
		t.Fatalf("wrong groups: %d", len(p.Completed))
	}
	if _, err = r.Exec(ctx, dir, "cat-file", "-e", "HEAD:b.txt"); err == nil {
		t.Fatal("rename source retained")
	}
	if _, err = r.Exec(ctx, dir, "cat-file", "-e", "HEAD:c.txt"); err == nil {
		t.Fatal("deletion retained")
	}
	info, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".local/state/vorq/plans", p.ID+".json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("journal permissions: %v", err)
	}
}

func TestAutomationHookContentMutationRequiresInspection(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	ctx := context.Background()
	r := newTestRunner(t)
	p, err := a.PlanCommitsAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	hookDir := filepath.Join(t.TempDir(), "hooks")
	os.MkdirAll(hookDir, 0700)
	// A native hook can change its private index: report the created commit and
	// refuse to claim it equals the reviewed content or retry the group.
	script := "#!/bin/sh\nprintf 'hook mutation\\n' > a.txt\ngit add -- a.txt\n"
	if err = os.WriteFile(filepath.Join(hookDir, "pre-commit"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Exec(ctx, dir, "config", "core.hooksPath", hookDir); err != nil {
		t.Fatal(err)
	}
	p, err = a.ExecuteCommitPlan(ctx, dir, p)
	if err == nil || len(p.Completed) != 1 || !strings.HasPrefix(p.Error, "INSPECT:") {
		t.Fatalf("mutation not reported: %v %+v", err, p)
	}
	if _, err = a.ExecuteCommitPlan(ctx, dir, p); err == nil {
		t.Fatal("inspection gate bypassed")
	}
}

func TestAutomationPlanEditsMoveOrderAndMessage(t *testing.T) {
	a, dir, _ := prepareAutomation(t)
	ctx := context.Background()
	p, err := a.PlanCommitsAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	id := p.Commits[0].Files[0]
	targetID := p.Commits[1].Files[0]
	p, err = a.EditCommitPlan(ctx, dir, p, domain.PlanEdit{Kind: "move", Group: 0, Target: 1, FileID: id})
	if err != nil || len(p.Commits) != 3 || len(p.Commits[0].Files) != 2 {
		t.Fatalf("move: %v", err)
	}
	p, err = a.EditCommitPlan(ctx, dir, p, domain.PlanEdit{Kind: "message", Group: 0, Subject: "fix: combine dependent changes", Body: "Reviewed together."})
	if err != nil {
		t.Fatal(err)
	}
	p, err = a.EditCommitPlan(ctx, dir, p, domain.PlanEdit{Kind: "order", Group: 0, Target: 2})
	if err != nil || p.Commits[2].Subject != "fix: combine dependent changes" || p.Commits[2].Files[0] != targetID {
		t.Fatalf("order: %v", err)
	}
	if err = a.ValidateCommitPlan(ctx, dir, p); err != nil {
		t.Fatal(err)
	}
}
