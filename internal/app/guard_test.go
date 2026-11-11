package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// Credentials are assembled at run time; none is ever a literal in the repo.
var (
	guardAWS  = strings.Join([]string{"AKIA", "Z7QWERTYUIOPLKJH"}, "")
	guardPass = strings.Join([]string{"Vq7", "#pL2", "zR9!", "mK4"}, "")
)

func blockedReport(t *testing.T, err error) domain.GuardReport {
	t.Helper()
	var blocked *domain.GuardBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("expected a guard block, got %v", err)
	}
	return blocked.Report
}

func TestGuard_CommitBlocksSecretsUntilTypedOverride(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "README.md", "ok\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial", false); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, ".env", "DB_PASSWORD="+guardPass+"\n")
	writeCommitFile(t, dir, "config.go", "package config\n\nconst key = \""+guardAWS+"\"\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	report, err := a.CheckCommit(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range report.Blocking() {
		got[f.Path+"|"+f.Rule] = true
		for _, secret := range []string{guardAWS, guardPass} {
			if strings.Contains(f.Excerpt, secret) || strings.Contains(f.Message, secret) {
				t.Fatalf("finding leaks the value: %+v", f)
			}
		}
	}
	for _, want := range []string{".env|sensitive-file", ".env|generic-secret", "config.go|aws-access-key"} {
		if !got[want] {
			t.Fatalf("missing %s in %+v", want, report.Findings)
		}
	}
	head, _ := r.RevParseHead(ctx, dir)
	_, err = a.CreateCommit(ctx, dir, "feat: add config", false)
	if blocked := blockedReport(t, err); len(blocked.Blocking()) != len(report.Blocking()) {
		t.Fatalf("core must block with the same findings: %+v", blocked)
	}
	diff, _ := r.DiffCached(ctx, dir)
	_, err = a.CreateReviewedCommit(ctx, dir, "feat: add config", diff, false)
	blockedReport(t, err)
	if after, _ := r.RevParseHead(ctx, dir); after != head {
		t.Fatal("a blocked commit must not create anything")
	}
	result, err := a.CreateReviewedCommit(ctx, dir, "feat: add config", diff, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Overridden) != len(report.Blocking()) {
		t.Fatalf("override must be recorded in the result: %+v", result)
	}
}

func TestGuard_WarningsAndExceptionsDoNotBlock(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "app.js", "console.log('x')\n")
	writeCommitFile(t, dir, "fixtures/creds.txt", "aws = "+guardAWS+"\n")
	writeCommitFile(t, dir, "docs.go", "package docs\n\nconst sample = \""+guardAWS+"\" // vorq:allow-secret\n")
	writeCommitFile(t, dir, ".vorqsecrets-allow", "# test fixtures\nfixtures/** aws-access-key\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	result, err := a.CreateCommit(ctx, dir, "chore: add fixtures", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Rule != "debug-statement" || len(result.Overridden) != 0 {
		t.Fatalf("only the debug warning is expected: %+v", result)
	}
}

func TestGuard_ConflictMarkersBlock(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "main.go", "package main\n<<<<<<< HEAD\nvar a = 1\n=======\nvar a = 2\n>>>>>>> topic\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	_, err := a.CreateCommit(ctx, dir, "fix: merge", false)
	if r := blockedReport(t, err); r.Blocking()[0].Rule != "conflict-marker" || r.Blocking()[0].Line != 2 {
		t.Fatalf("conflict marker at main.go:2 expected: %+v", r)
	}
}

func TestGuard_PushEvaluatesEveryUnpublishedCommit(t *testing.T) {
	a, r, dir, remote := setupPushApp(t)
	ctx := context.Background()
	// The secret is added and then removed: it still lives in history.
	writeCommitFile(t, dir, "deploy.sh", "export AWS_KEY="+guardAWS+"\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: deploy", true); err != nil {
		t.Fatal(err)
	}
	leaked, _ := r.RevParseHead(ctx, dir)
	writeCommitFile(t, dir, "deploy.sh", "export AWS_KEY=$AWS_KEY\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "fix: read key from env", false); err != nil {
		t.Fatal(err)
	}
	target, err := a.GetPushTarget(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.Guard == nil || target.Guard.Commits != 3 {
		t.Fatalf("all unpublished commits (no upstream yet) must be evaluated: %+v", target.Guard)
	}
	blocking := target.Guard.Blocking()
	if len(blocking) != 1 || blocking[0].Commit != leaked || blocking[0].Path != "deploy.sh" || blocking[0].Line != 1 {
		t.Fatalf("the removed secret must be found in its commit: %+v", blocking)
	}
	_, err = a.PushChanges(ctx, dir, domain.PushOptions{Target: *target})
	blockedReport(t, err)
	if _, err := r.Exec(ctx, remote, "show-ref", "--verify", "refs/heads/main"); err == nil {
		t.Fatal("a blocked push must publish nothing")
	}
	result, err := a.PushChanges(ctx, dir, domain.PushOptions{Target: *target, AllowFindings: true})
	if err != nil || len(result.Overridden) != 1 {
		t.Fatalf("typed override must push and be recorded: %+v %v", result, err)
	}
	target, err = a.GetPushTarget(ctx, dir)
	if err != nil || target.Guard.Commits != 0 || target.Guard.HasBlocking() {
		t.Fatalf("published commits are not evaluated again: %+v %v", target.Guard, err)
	}
}

func TestGuard_SuggestionNeverSendsSecretFiles(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "README.md", "ok\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial", false); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "service.go", "package service\n\nfunc Run() {}\n")
	writeCommitFile(t, dir, "keys.txt", "aws "+guardAWS+"\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	var sent string
	a.SetAIProvider(aiFunc(func(_ context.Context, r domain.AIRequest) (*domain.CommitProposal, error) {
		sent = r.Diff
		return &domain.CommitProposal{Subject: "feat: add service"}, nil
	}))
	p, err := a.SuggestCommitAI(ctx, dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sent, guardAWS) || strings.Contains(sent, "keys.txt") || !strings.Contains(sent, "service.go") {
		t.Fatalf("payload must hold only the safe file: %q", sent)
	}
	if len(p.Withheld) != 1 || p.Withheld[0] != "keys.txt" {
		t.Fatalf("withheld files must be reported: %+v", p.Withheld)
	}
	if err = a.StageFiles(ctx, dir, []string{"-A"}); err != nil {
		t.Fatal(err)
	}
	if err = a.UnstageFiles(ctx, dir, []string{"service.go"}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.SuggestCommitAI(ctx, dir, "codex"); err == nil || !strings.Contains(err.Error(), "nothing sent") {
		t.Fatalf("only secret files staged: nothing may be sent, got %v", err)
	}
}

type capturingAI struct {
	aiFunc
	payload string
}

func (c *capturingAI) Analyze(_ context.Context, r domain.AnalysisRequest) (json.RawMessage, error) {
	c.payload = r.Payload
	return json.RawMessage(`{"summary":"ok","findings":[]}`), nil
}

func TestGuard_AnalysisExcludesSecretFilesWhole(t *testing.T) {
	a, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "README.md", "ok\n")
	writeCommitFile(t, dir, "settings.py", "DEBUG = False\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial", false); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "settings.py", "DEBUG = False\npassword = \""+guardPass+"\"\n")
	writeCommitFile(t, dir, "notes.md", "plain notes\n")
	ai := &capturingAI{}
	a.SetAIProvider(ai)
	review, err := a.ReviewDiffAI(ctx, dir, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ai.payload, guardPass) || strings.Contains(ai.payload, "settings.py") || !strings.Contains(ai.payload, "notes.md") {
		t.Fatalf("payload must exclude the secret file whole: %s", ai.payload)
	}
	for _, f := range review.Snapshot.Files {
		if f.Path == "settings.py" && (f.Excluded == "" || f.Content != nil || f.Diff != "") {
			t.Fatalf("withheld file must be excluded and emptied: %+v", f)
		}
	}
}

func TestCLI_GuardOverride(t *testing.T) {
	binary := t.TempDir() + "/vorq"
	if out, err := execCommand("go", "build", "-o", binary, "../../cmd/vorq"); err != nil {
		t.Fatalf("build %v: %s", err, out)
	}
	for _, tt := range []struct {
		name, input string
		args        []string
		commit      bool
		wantErr     bool
		wantText    string
	}{
		{"blocked without flag", "", []string{"-m", "feat: x"}, false, true, "--allow-secrets"},
		{"typed override", "CONFIRMAR\n", []string{"-m", "feat: x", "--allow-secrets"}, true, false, "Override registrado"},
		{"wrong word cancels", "confirmar\n", []string{"-m", "feat: x", "--allow-secrets"}, false, true, "Override não confirmado"},
		{"eof cancels", "", []string{"-m", "feat: x", "--allow-secrets"}, false, true, "interrupted"},
		{"scripts need both flags", "", []string{"-m", "feat: x", "--yes"}, false, true, "blocked"},
		{"script override", "", []string{"-m", "feat: x", "--allow-secrets", "--yes"}, true, false, "Override registrado"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, r, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, "keys.txt", "aws "+guardAWS+"\n")
			if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
				t.Fatal(err)
			}
			out, err := execIn(dir, tt.input, binary, append([]string{"commit"}, tt.args...)...)
			if (err != nil) != tt.wantErr || !strings.Contains(out, tt.wantText) || !strings.Contains(out, "AKIA…LKJH") || strings.Contains(out, guardAWS) {
				t.Fatalf("err=%v output:\n%s", err, out)
			}
			if _, err := r.RevParseHead(ctx, dir); (err == nil) != tt.commit {
				t.Fatalf("commit created=%v, want %v\n%s", err == nil, tt.commit, out)
			}
		})
	}
	t.Run("push", func(t *testing.T) {
		a, r, dir, remote := setupPushApp(t)
		ctx := context.Background()
		writeCommitFile(t, dir, "keys.txt", "aws "+guardAWS+"\n")
		if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
			t.Fatal(err)
		}
		if _, err := a.CreateCommit(ctx, dir, "chore: keys", true); err != nil {
			t.Fatal(err)
		}
		out, err := execIn(dir, "\n", binary, "push", "--yes")
		if err == nil || !strings.Contains(out, "git commit --amend") || !strings.Contains(out, "2 commit(s) não publicados avaliados") {
			t.Fatalf("push must be blocked with remediation: %v\n%s", err, out)
		}
		if _, err := r.Exec(ctx, remote, "show-ref", "--verify", "refs/heads/main"); err == nil {
			t.Fatal("blocked push published")
		}
		if out, err = execIn(dir, "CONFIRMAR\n", binary, "push", "--allow-secrets"); err != nil || !strings.Contains(out, "Override registrado") {
			t.Fatalf("typed override must push: %v\n%s", err, out)
		}
		if _, err := r.Exec(ctx, remote, "show-ref", "--verify", "refs/heads/main"); err != nil {
			t.Fatal("override push did not publish")
		}
	})
}

func execCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

func execIn(dir, input, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestGuard_LargeFileAndCaseCollisionWarn(t *testing.T) {
	a, r, dir := setupCommitApp(t)
	ctx := context.Background()
	if _, err := r.Exec(ctx, dir, "config", "core.ignorecase", "false"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "Readme.md", "a\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateCommit(ctx, dir, "chore: initial", false); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "data/big.txt", strings.Repeat("v", 5*1024*1024+1))
	writeCommitFile(t, dir, "README.md", "b\n")
	if err := a.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	report, err := a.CheckCommit(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]string{}
	for _, f := range report.Findings {
		rules[f.Rule] = f.Path
	}
	if report.HasBlocking() || rules["large-file"] != "data/big.txt" || rules["case-conflict"] != "README.md" {
		t.Fatalf("large file and case collision must warn only: %+v", report.Findings)
	}
}
