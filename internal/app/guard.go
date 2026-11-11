package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/secrets"
)

// VORQ-018: local guardrails shared by every commit path (manual, CLI,
// reviewed, plan), every AI payload and the push. One core, no bypass by
// interface: an override is explicit, typed by the user and per operation.

// MaxPushCommits bounds how many unpublished commits a push inspects.
const MaxPushCommits = 500

const maxAllowlistBytes = 64 * 1024

// guardScanner reads the versioned exceptions from the workspace root. A
// missing file means no exceptions; a malformed one is an error (fail closed).
func guardScanner(root string) (*secrets.Scanner, error) {
	fs, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	f, err := fs.Open(secrets.AllowlistFile)
	if os.IsNotExist(err) {
		return secrets.NewScanner("")
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAllowlistBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAllowlistBytes {
		return nil, fmt.Errorf("%s exceeds %d KiB", secrets.AllowlistFile, maxAllowlistBytes/1024)
	}
	return secrets.NewScanner(string(data))
}

func sortFindings(fs []domain.GuardFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Blocking != fs[j].Blocking {
			return fs[i].Blocking
		}
		if fs[i].Path != fs[j].Path {
			return fs[i].Path < fs[j].Path
		}
		return fs[i].Line < fs[j].Line
	})
}

// CheckCommit evaluates exactly what the next commit would record: the
// staged patch, staged sizes and case collisions with the index.
func (a *App) CheckCommit(ctx context.Context, dir string) (*domain.GuardReport, error) {
	cs, err := a.GetCommitChanges(ctx, dir)
	if err != nil {
		return nil, err
	}
	return a.checkStaged(ctx, cs)
}

func (a *App) checkStaged(ctx context.Context, cs *domain.ChangeSet) (*domain.GuardReport, error) {
	s, err := guardScanner(cs.WorkspacePath)
	if err != nil {
		return nil, err
	}
	report := &domain.GuardReport{Findings: s.ScanPatch(cs.StagedDiff)}
	sizes, err := a.gitRunner.StagedBlobSizes(ctx, cs.WorkspacePath)
	if err != nil {
		return nil, err
	}
	var changed []string
	for p, size := range sizes {
		changed = append(changed, p)
		if f := s.LargeFile(p, size); f != nil {
			report.Findings = append(report.Findings, *f)
		}
	}
	sort.Strings(changed)
	if len(changed) > 0 {
		indexed, err := a.gitRunner.IndexedPaths(ctx, cs.WorkspacePath)
		if err != nil {
			return nil, err
		}
		report.Findings = append(report.Findings, s.CaseConflicts(indexed, changed)...)
	}
	sortFindings(report.Findings)
	return report, nil
}

// checkUnpublished evaluates every commit not yet on any remote, one by one,
// so a secret added and later removed is still caught before it is public.
// Only blocking rules matter here; warnings were shown at commit time.
func (a *App) checkUnpublished(ctx context.Context, root string) (*domain.GuardReport, error) {
	s, err := guardScanner(root)
	if err != nil {
		return nil, err
	}
	commits, err := a.gitRunner.UnpublishedCommits(ctx, root, MaxPushCommits)
	if err != nil {
		return nil, err
	}
	report := &domain.GuardReport{}
	if len(commits) > MaxPushCommits {
		report.Commits = MaxPushCommits
		report.Findings = append(report.Findings, domain.GuardFinding{Rule: "push-unverified", Category: domain.GuardUnverified, Blocking: true,
			Message: fmt.Sprintf("more than %d unpublished commits; only the latest %d were evaluated", MaxPushCommits, MaxPushCommits)})
		commits = commits[:MaxPushCommits]
	}
	for _, hash := range commits {
		patch, err := a.gitRunner.CommitPatch(ctx, root, hash)
		if err != nil {
			return nil, err
		}
		report.Commits++
		for _, f := range s.ScanPatch(patch) {
			if f.Blocking {
				f.Commit = hash
				report.Findings = append(report.Findings, f)
			}
		}
	}
	return report, nil
}

// PushRemediation explains how to remove a committed secret; vorq never
// rewrites history by itself.
func PushRemediation(r *domain.GuardReport) []string {
	if !r.HasBlocking() {
		return nil
	}
	// User guidance (shown by CLI and TUI), so in Portuguese like other labels.
	return []string{
		"Último commit: git rm --cached <arquivo> (ou edite o arquivo) e depois git commit --amend",
		"Commit anterior: git rebase -i <commit>^, marque-o como edit, corrija, git commit --amend, git rebase --continue",
		"Segredo que já foi exposto deve ser revogado/trocado no provedor",
		"Falso positivo: comentário \"" + secrets.InlineAllow + "\" na linha ou caminho/regra em " + secrets.AllowlistFile,
	}
}

// withheldFromAI decides, per file, whether text may be sent to the AI. Every
// line counts (added, removed and context lines are all sent), plus the path
// rules: a blocking secret keeps the whole file out.
func withheldFromAI(s *secrets.Scanner, path, text string) bool {
	return s.PathFinding(path) != nil || secrets.HasSecret(s.ScanContent(path, text))
}

// splitPatch cuts a multi-file Git patch at each "diff --git" header.
func splitPatch(patch string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(patch); {
		next := strings.Index(patch[i:], "\ndiff --git ")
		if next < 0 {
			break
		}
		i += next + 1
		parts = append(parts, patch[start:i])
		start = i
	}
	if start < len(patch) {
		parts = append(parts, patch[start:])
	}
	return parts
}

// filterPatchForAI drops each file section that holds a secret and returns
// the paths withheld.
func filterPatchForAI(s *secrets.Scanner, patch string) (string, []string) {
	var kept strings.Builder
	var withheld []string
	for _, part := range splitPatch(patch) {
		if path := secrets.PatchPath(part); withheldFromAI(s, path, part) {
			withheld = append(withheld, path)
			continue
		}
		kept.WriteString(part)
	}
	return kept.String(), withheld
}

// WithheldReason marks a snapshot file kept out of the AI payload.
const WithheldReason = "possible secret: not sent to AI"
