package app_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func setupCommitApp(t *testing.T) (*app.App, *git.NativeRunner, string) {
	t.Helper()
	dir := t.TempDir()
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Commit Test"},
		{"config", "user.email", "commit@example.com"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", filepath.Join(t.TempDir(), "hooks")},
	} {
		if _, err := runner.Exec(context.Background(), dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return app.NewApp(runner), runner, dir
}

func writeCommitFile(t *testing.T, dir, path, content string) {
	t.Helper()
	target := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestApp_StageFiles_And_Commit(t *testing.T) {
	application, runner, dir := setupCommitApp(t)
	ctx := context.Background()
	path := "internal/git/ação [literal].go"
	writeCommitFile(t, dir, path, "staged version\n")
	writeCommitFile(t, dir, "other.txt", "leave untracked\n")
	// All paths are relative to the workspace root, even from a nested cwd.
	nested := filepath.Join(dir, "internal/git")
	if err := application.StageFiles(ctx, nested, []string{path, "other.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := application.UnstageFiles(ctx, nested, []string{"other.txt"}); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, path, "unstaged version\n")
	cs, err := application.GetCommitChanges(ctx, nested)
	if err != nil {
		t.Fatal(err)
	}
	if cs.WorkspacePath != dir || cs.StagedCount() != 1 || cs.UntrackedCount() != 1 || !strings.Contains(cs.StagedDiff, "+staged version") || strings.Contains(cs.StagedDiff, "unstaged version") {
		t.Fatalf("unexpected index: %+v", cs)
	}
	proposal, err := application.ProposeCommit(ctx, nested, "feat")
	if err != nil || proposal.Message() != "feat(git): add ação [literal].go" {
		t.Fatalf("proposal %+v: %v", proposal, err)
	}
	result, err := application.CreateCommit(ctx, nested, proposal.Message())
	if err != nil {
		t.Fatal(err)
	}
	head, err := runner.RevParseHead(ctx, dir)
	if err != nil || result.Hash != head || len(result.Hash) < 40 || result.ShortHash != head[:7] || result.Subject != proposal.Message() || !reflect.DeepEqual(result.FilesCommitted, []string{path}) {
		t.Fatalf("result %+v, HEAD %q: %v", result, head, err)
	}
	content, err := runner.Exec(ctx, dir, "show", "HEAD:"+path)
	if err != nil || content != "staged version\n" {
		t.Fatalf("committed unstaged content: %q: %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "other.txt")); err != nil {
		t.Fatal("untracked file lost")
	}
}

func TestApp_CommitValidation(t *testing.T) {
	application, runner, dir := setupCommitApp(t)
	ctx := context.Background()
	if _, err := application.CreateCommit(ctx, dir, "feat: empty"); err == nil {
		t.Fatal("accepted empty index")
	}
	writeCommitFile(t, dir, "file.txt", "data")
	if err := application.StageFiles(ctx, dir, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateCommit(ctx, dir, " \n\t"); err == nil {
		t.Fatal("accepted empty message")
	}
	if _, err := application.ProposeCommit(ctx, dir, "invalid"); err == nil {
		t.Fatal("accepted invalid type")
	}
	for _, paths := range [][]string{nil, {""}, {"../outside"}, {filepath.Join(dir, "file.txt")}} {
		if err := application.StageFiles(ctx, dir, paths); err == nil {
			t.Fatalf("accepted invalid paths: %v", paths)
		}
		if err := application.UnstageFiles(ctx, dir, paths); err == nil {
			t.Fatalf("accepted invalid unstage: %v", paths)
		}
	}
	if _, err := runner.RevParseHead(ctx, dir); err == nil {
		t.Fatal("validation created a commit")
	}
}

func TestApp_Commit_CurrentLinkedWorktree(t *testing.T) {
	mainDir, secondDir := setupTestGitRepo(t)
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	application := app.NewApp(runner)
	ctx := context.Background()
	before, err := runner.RevParseHead(ctx, mainDir)
	if err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, secondDir, "nested/selected.txt", "linked worktree\n")
	nested := filepath.Join(secondDir, "nested")
	if err := application.StageFiles(ctx, nested, []string{"nested/selected.txt"}); err != nil {
		t.Fatal(err)
	}
	result, err := application.CreateCommit(ctx, nested, "feat: linked worktree")
	if err != nil {
		t.Fatal(err)
	}
	after, err := runner.RevParseHead(ctx, mainDir)
	if err != nil || before != after || !reflect.DeepEqual(result.FilesCommitted, []string{"nested/selected.txt"}) {
		t.Fatalf("wrong worktree: %+v, main %s -> %s: %v", result, before, after, err)
	}
}

func TestApp_Commit_BlocksConflicts(t *testing.T) {
	application, runner, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "file.txt", "base\n")
	if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateCommit(ctx, dir, "chore: base"); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "checkout", "-b", "side")
	writeCommitFile(t, dir, "file.txt", "side\n")
	if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateCommit(ctx, dir, "fix: side"); err != nil {
		t.Fatal(err)
	}
	runCmd(t, dir, "git", "checkout", "main")
	writeCommitFile(t, dir, "file.txt", "main\n")
	if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateCommit(ctx, dir, "fix: main"); err != nil {
		t.Fatal(err)
	}
	before, err := runner.RevParseHead(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Exec(ctx, dir, "merge", "side"); err == nil {
		t.Fatal("expected fixture conflict")
	}
	if _, err := application.ProposeCommit(ctx, dir, "fix"); err == nil || !strings.Contains(err.Error(), "resolve merge conflicts") {
		t.Fatalf("proposal accepted conflict: %v", err)
	}
	if _, err := application.CreateCommit(ctx, dir, "fix: conflict"); err == nil {
		t.Fatal("commit accepted conflict")
	}
	after, err := runner.RevParseHead(ctx, dir)
	if err != nil || before != after {
		t.Fatal("conflict changed HEAD")
	}
	writeCommitFile(t, dir, "file.txt", "resolved\n")
	if err := application.StageFiles(ctx, dir, []string{"file.txt"}); err != nil {
		t.Fatal(err)
	}
	result, err := application.CreateCommit(ctx, dir, "fix: resolve merge")
	if err != nil || !reflect.DeepEqual(result.FilesCommitted, []string{"file.txt"}) {
		t.Fatalf("merge metadata %+v: %v", result, err)
	}
}

func TestApp_PreviewCommit_Passive(t *testing.T) {
	for _, initialCommit := range []bool{false, true} {
		name := "unborn"
		if initialCommit {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			application, runner, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, ".gitignore", ".env\n")
			writeCommitFile(t, dir, "docs/ação.md", "first\n")
			if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
				t.Fatal(err)
			}
			if initialCommit {
				if _, err := application.CreateCommit(ctx, dir, "chore: initial"); err != nil {
					t.Fatal(err)
				}
				writeCommitFile(t, dir, "docs/ação.md", "changed\n")
			}
			writeCommitFile(t, dir, "nested/new file.txt", "new\n")
			writeCommitFile(t, dir, ".env", "private\n")
			indexPath := filepath.Join(dir, ".git", "index")
			before, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			statusBefore, err := runner.Status(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			preview, proposal, err := application.PreviewCommit(ctx, dir, "feat", true)
			if err != nil {
				t.Fatal(err)
			}
			if proposal == nil || preview.StagedCount() < 2 {
				t.Fatalf("unexpected preview %+v %+v", preview, proposal)
			}
			for _, f := range preview.Files {
				if f.Path == ".env" {
					t.Fatal("ignored file included")
				}
			}
			after, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			statusAfter, err := runner.Status(ctx, dir)
			if err != nil || statusBefore != statusAfter || !bytes.Equal(before, after) {
				t.Fatalf("preview changed real index: %v", err)
			}
		})
	}
}

func TestApp_PreviewCommit_StagedRevertedInWorktree(t *testing.T) {
	application, _, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "file.txt", "original\n")
	if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.CreateCommit(ctx, dir, "chore: initial"); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "file.txt", "staged\n")
	if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	writeCommitFile(t, dir, "file.txt", "original\n")
	preview, _, err := application.PreviewCommit(ctx, dir, "fix", true)
	if err == nil || preview.StagedCount() != 0 {
		t.Fatalf("-a would have no net change: %+v, %v", preview, err)
	}
	proposal, err := application.ProposeCommit(ctx, dir, "fix")
	if err != nil || proposal.Message() != "fix: update file.txt" {
		t.Fatalf("index proposal lost: %+v %v", proposal, err)
	}
}

func TestApp_PreviewCommit_DeletedThenRecreated(t *testing.T) {
	for _, content := range []string{"original\n", "changed\n"} {
		t.Run(strings.TrimSpace(content), func(t *testing.T) {
			application, runner, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, "file.txt", "original\n")
			if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
				t.Fatal(err)
			}
			if _, err := application.CreateCommit(ctx, dir, "chore: initial"); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dir, "file.txt")); err != nil {
				t.Fatal(err)
			}
			if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
				t.Fatal(err)
			}
			writeCommitFile(t, dir, "file.txt", content)
			before, err := runner.Status(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			preview, _, err := application.PreviewCommit(ctx, dir, "fix", true)
			if content == "original\n" {
				if err == nil || len(preview.Files) != 0 {
					t.Fatalf("expected no net change, got %+v: %v", preview, err)
				}
			} else if err != nil || len(preview.Files) != 1 || preview.Files[0].Staged != "M" {
				t.Fatalf("expected M, got %+v: %v", preview, err)
			}
			after, err := runner.Status(ctx, dir)
			if err != nil || before != after {
				t.Fatal("preview modified the index")
			}
		})
	}
}

func TestApp_Commit_HookMetadata(t *testing.T) {
	application, runner, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "file.txt", "data")
	if err := application.StageFiles(ctx, dir, []string{"."}); err != nil {
		t.Fatal(err)
	}
	hooks := t.TempDir()
	if _, err := runner.Exec(ctx, dir, "config", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "commit-msg"), []byte("#!/bin/sh\nprintf 'fix: hook edited message\\n' > \"$1\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := application.CreateCommit(ctx, dir, "feat: before hook")
	if err != nil || result.Subject != "fix: hook edited message" {
		t.Fatalf("result %+v: %v", result, err)
	}
}

// CLI tests live here to keep the slice's file boundary. The actual binary runs
// with stdin/stdout pipes in t.TempDir repositories, just like a human/script.
func TestCLI_Commit(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "vorq")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/vorq")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, tt := range []struct {
		name       string
		args       []string
		input      string
		stage      bool
		clean      bool
		wantError  bool
		wantCommit bool
		wantStage  int
		wantText   string
	}{
		{"clean", nil, "", false, true, false, false, 0, "Working tree clean"},
		{"default confirm", nil, "1\n\n", true, false, false, true, 0, "✓ Commit criado:"},
		{"type cancel", []string{"-t", "fix"}, "n\n", true, false, false, false, 1, "Os arquivos permanecem em stage."},
		{"type EOF", []string{"-t", "fix"}, "", true, false, true, false, 1, "entrada interativa interrompida"},
		{"invalid confirmation then cancel", []string{"-t", "fix"}, "invalid\nn\n", true, false, false, false, 1, "Opção inválida"},
		{"edit", []string{"-t", "feat"}, "e\n\nfix: edited\n", true, false, false, true, 0, "fix: edited"},
		{"manual", []string{"-m", "docs: explicit"}, "", true, false, false, true, 0, "docs: explicit"},
		{"all manual", []string{"-a", "-m", "feat: all files"}, "", false, false, false, true, 0, "feat: all files"},
		{"manual requires stage", []string{"-m", "feat: explicit"}, "", false, false, true, false, 0, "nenhum arquivo em stage"},
		{"empty manual", []string{"-a", "-m", " "}, "", false, false, true, false, 0, "não pode estar vazia"},
		{"invalid type before stage", []string{"-a", "-t", "other"}, "", false, false, true, false, 0, "invalid commit type"},
		{"stage all then cancel", nil, "1\n1\nn\n", false, false, false, false, 2, "Os arquivos permanecem em stage."},
		{"stage selective", nil, "2\n0\n1,1\n1\ny\n", false, false, false, true, 0, "Seleção inválida"},
		{"literal sentinel", nil, "2\n1\n1\ny\n", false, false, false, true, 0, "✓ Commit criado:"},
		{"cancel stage", nil, "3\n", false, false, false, false, 0, "Commit cancelado pelo usuário."},
		{"dry staged", []string{"--dry-run", "-t", "feat"}, "", true, false, false, false, 1, "[DRY-RUN] Proposta de commit: feat"},
		{"dry all passive", []string{"--dry-run", "-a", "-t", "feat"}, "", false, false, false, false, 0, "[DRY-RUN] Proposta de commit: feat"},
		{"dry manual", []string{"--dry-run", "-a", "-m", "docs: dry"}, "", false, false, false, false, 0, "[DRY-RUN] Proposta de commit: docs: dry"},
		{"dry default type", []string{"--dry-run"}, "", true, false, false, false, 1, "[DRY-RUN] Proposta de commit: chore"},
		{"dry no stage", []string{"--dry-run"}, "", false, false, true, false, 0, "no staged files"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			application, runner, dir := setupCommitApp(t)
			ctx := context.Background()
			if !tt.clean {
				writeCommitFile(t, dir, "a ação.txt", "first\n")
				writeCommitFile(t, dir, "nested/b.txt", "second\n")
			}
			if tt.name == "literal sentinel" {
				writeCommitFile(t, dir, "-A", "literal\n")
			}
			if tt.stage {
				if err := application.StageFiles(ctx, dir, []string{"a ação.txt"}); err != nil {
					t.Fatal(err)
				}
			}
			indexPath := filepath.Join(dir, ".git/index")
			before, beforeErr := os.ReadFile(indexPath)
			cmd := exec.Command(binary, append([]string{"commit"}, tt.args...)...)
			cmd.Dir = dir
			cmd.Stdin = strings.NewReader(tt.input)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tt.wantError || !strings.Contains(string(out), tt.wantText) {
				t.Fatalf("error=%v, output:\n%s", err, out)
			}
			_, headErr := runner.RevParseHead(ctx, dir)
			if (headErr == nil) != tt.wantCommit {
				t.Fatalf("unexpected HEAD: %v\n%s", headErr, out)
			}
			cs, err := application.GetCommitChanges(ctx, dir)
			if err != nil || cs.StagedCount() != tt.wantStage {
				t.Fatalf("stage %+v: %v", cs, err)
			}
			if tt.name == "stage selective" {
				files, err := runner.Exec(ctx, dir, "ls-tree", "--name-only", "-r", "HEAD")
				if err != nil || strings.Contains(files, "nested") {
					t.Fatalf("selection leaked: %q %v", files, err)
				}
			}
			if tt.name == "literal sentinel" {
				files, err := runner.Exec(ctx, dir, "ls-tree", "--name-only", "-r", "HEAD")
				if err != nil || strings.TrimSpace(files) != "-A" {
					t.Fatalf("literal selection staged extra files: %q %v", files, err)
				}
			}
			if strings.Contains(tt.name, "dry") {
				after, afterErr := os.ReadFile(indexPath)
				if (beforeErr == nil) != (afterErr == nil) || !bytes.Equal(before, after) {
					t.Fatal("dry-run mutated or created the real index")
				}
			}
		})
	}
	help := exec.Command(binary, "commit", "--help")
	if out, err := help.CombinedOutput(); err != nil {
		t.Fatal(err)
	} else {
		for _, flag := range []string{"--all", "--type", "--message", "--dry-run"} {
			if !strings.Contains(string(out), flag) {
				t.Fatalf("missing %s in help", flag)
			}
		}
	}
}

func setupTestGitRepo(t *testing.T) (string, string) {
	t.Helper()

	mainDir := t.TempDir()
	runCmd(t, mainDir, "git", "init", "-b", "main")
	runCmd(t, mainDir, "git", "config", "user.name", "Test User")
	runCmd(t, mainDir, "git", "config", "user.email", "test@example.com")

	// Commit initial file
	dummyFile := filepath.Join(mainDir, "README.md")
	if err := os.WriteFile(dummyFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}
	runCmd(t, mainDir, "git", "add", "README.md")
	runCmd(t, mainDir, "git", "commit", "-m", "initial commit")

	// Create second worktree
	secondDir := filepath.Join(t.TempDir(), "wt-feature")
	runCmd(t, mainDir, "git", "worktree", "add", "-b", "feat/test", secondDir, "main")

	// Add dirty uncommitted file in second worktree
	dirtyFile := filepath.Join(secondDir, "dirty.txt")
	if err := os.WriteFile(dirtyFile, []byte("changes\n"), 0644); err != nil {
		t.Fatalf("failed to write dirty file: %v", err)
	}

	return mainDir, secondDir
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed [%s %v]: %v\nOutput: %s", name, args, err, string(out))
	}
}

func TestApp_ResolveProject(t *testing.T) {
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatalf("failed to initialize NativeRunner: %v", err)
	}

	application := app.NewApp(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	proj, err := application.ResolveProject(ctx, ".")
	if err != nil {
		t.Fatalf("expected ResolveProject to succeed in git repository, got: %v", err)
	}

	if proj.RootPath == "" {
		t.Errorf("expected non-empty RootPath")
	}

	if !strings.Contains(proj.GitDir, ".git") {
		t.Errorf("expected GitDir to contain .git, got: %q", proj.GitDir)
	}
}

func TestApp_ListWorktrees_And_Status(t *testing.T) {
	mainDir, secondDir := setupTestGitRepo(t)

	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatalf("failed to initialize NativeRunner: %v", err)
	}

	application := app.NewApp(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Test ListWorktrees from mainDir
	worktrees, err := application.ListWorktrees(ctx, mainDir)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(worktrees))
	}

	// Main worktree checks
	if !worktrees[0].IsMain {
		t.Errorf("expected worktree[0] to be main")
	}
	if !worktrees[0].IsCurrent {
		t.Errorf("expected worktree[0] to be marked as current when called from mainDir")
	}
	if worktrees[0].Status == nil || !worktrees[0].Status.IsClean() {
		t.Errorf("expected worktree[0] to be clean")
	}

	// Second worktree checks
	if worktrees[1].IsMain {
		t.Errorf("expected worktree[1] not to be main")
	}
	if worktrees[1].Branch != "feat/test" {
		t.Errorf("expected branch 'feat/test', got %q", worktrees[1].Branch)
	}
	if worktrees[1].Status == nil || worktrees[1].Status.UntrackedCount() != 1 {
		t.Errorf("expected 1 untracked file in second worktree, got %+v", worktrees[1].Status)
	}

	// 2. Test GetWorkspaceStatus for secondDir
	ws, cs, err := application.GetWorkspaceStatus(ctx, mainDir, "feat/test")
	if err != nil {
		t.Fatalf("GetWorkspaceStatus failed: %v", err)
	}

	if ws.Branch != "feat/test" {
		t.Errorf("expected branch 'feat/test', got %q", ws.Branch)
	}
	if cs.UntrackedCount() != 1 {
		t.Errorf("expected 1 untracked file, got %d", cs.UntrackedCount())
	}
	if cs.Files[0].Path != "dirty.txt" {
		t.Errorf("expected dirty.txt, got %q", cs.Files[0].Path)
	}

	_ = secondDir
}

func TestApp_SpawnWorkspace(t *testing.T) {
	mainDir, _ := setupTestGitRepo(t)

	// Add .env and .env.local to mainDir
	_ = os.WriteFile(filepath.Join(mainDir, ".env"), []byte("FOO=bar\n"), 0644)
	_ = os.WriteFile(filepath.Join(mainDir, ".env.local"), []byte("LOCAL=secret\n"), 0600)
	_ = os.WriteFile(filepath.Join(mainDir, "extra.conf"), []byte("EXTRA=1\n"), 0644)

	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatalf("failed to initialize NativeRunner: %v", err)
	}

	application := app.NewApp(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Successful spawn
	res, err := application.SpawnWorkspace(ctx, mainDir, app.SpawnOptions{
		BranchName: "feat/spawn-test",
		BaseBranch: "main",
		CopyFiles:  []string{"extra.conf"},
	})
	if err != nil {
		t.Fatalf("SpawnWorkspace failed: %v", err)
	}

	if res.Workspace.Branch != "feat/spawn-test" {
		t.Errorf("expected branch 'feat/spawn-test', got %q", res.Workspace.Branch)
	}

	// Verify that destination folder was created and has files
	if _, err := os.Stat(res.Workspace.Path); err != nil {
		t.Errorf("destination workspace path does not exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(res.Workspace.Path, ".env")); err != nil {
		t.Errorf(".env was not copied to workspace")
	}
	if _, err := os.Stat(filepath.Join(res.Workspace.Path, ".env.local")); err != nil {
		t.Errorf(".env.local was not copied to workspace")
	}
	if _, err := os.Stat(filepath.Join(res.Workspace.Path, "extra.conf")); err != nil {
		t.Errorf("extra.conf was not copied to workspace")
	}

	// 2. Collision: Try to spawn the exact same branch again without --checkout
	_, err = application.SpawnWorkspace(ctx, mainDir, app.SpawnOptions{
		BranchName: "feat/spawn-test",
		BaseBranch: "main",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected collision error for existing branch, got: %v", err)
	}

	// 3. Reuse branch with CheckoutExisting:
	// First, create a branch that is not currently checked out anywhere
	runCmd(t, mainDir, "git", "branch", "feat/unattached", "main")

	customDir := filepath.Join(t.TempDir(), "wt-checkout-reuse")
	resCheckout, err := application.SpawnWorkspace(ctx, mainDir, app.SpawnOptions{
		BranchName:       "feat/unattached",
		TargetDir:        customDir,
		CheckoutExisting: true,
	})
	if err != nil {
		t.Fatalf("unexpected checkout error: %v", err)
	}
	if resCheckout.Workspace.Path != customDir {
		t.Errorf("expected customDir %q, got %q", customDir, resCheckout.Workspace.Path)
	}
	if resCheckout.Workspace.Branch != "feat/unattached" {
		t.Errorf("expected branch 'feat/unattached', got %q", resCheckout.Workspace.Branch)
	}
}

func TestApp_RemoveWorkspace_Safety(t *testing.T) {
	mainDir, secondDir := setupTestGitRepo(t)

	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatalf("failed to initialize NativeRunner: %v", err)
	}

	application := app.NewApp(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Guard against removing main workspace
	_, err = application.RemoveWorkspace(ctx, mainDir, "main", true)
	if err == nil || !strings.Contains(err.Error(), "main working tree") {
		t.Errorf("expected error removing main workspace, got: %v", err)
	}

	// 2. Guard against removing active workspace (when called from secondDir)
	_, err = application.RemoveWorkspace(ctx, secondDir, "feat/test", true)
	if err == nil || !strings.Contains(err.Error(), "active workspace") {
		t.Errorf("expected error removing active workspace, got: %v", err)
	}

	// 3. Guard against removing secondary workspace with dirty changes (secondDir has dirty.txt)
	// From mainDir, attempt to remove secondDir without force
	_, err = application.RemoveWorkspace(ctx, mainDir, "feat/test", false)
	if err == nil || !strings.Contains(err.Error(), "safety check blocked removal") {
		t.Errorf("expected safety blocked error for dirty workspace, got: %v", err)
	}

	// 4. Guard against removing secondary workspace with ignored files
	// Clean the untracked dirty.txt, but add an ignored file .env
	_ = os.Remove(filepath.Join(secondDir, "dirty.txt"))
	_ = os.WriteFile(filepath.Join(secondDir, ".gitignore"), []byte(".env*\n"), 0644)
	runCmd(t, secondDir, "git", "add", ".gitignore")
	runCmd(t, secondDir, "git", "commit", "-m", "add gitignore")
	_ = os.WriteFile(filepath.Join(secondDir, ".env"), []byte("SECRET=123\n"), 0600)

	_, err = application.RemoveWorkspace(ctx, mainDir, "feat/test", false)
	if err == nil || !strings.Contains(err.Error(), "safety check blocked removal") {
		t.Errorf("expected safety blocked error for ignored file, got: %v", err)
	}

	// 5. Successful removal with --force
	report, err := application.RemoveWorkspace(ctx, mainDir, "feat/test", true)
	if err != nil {
		t.Fatalf("failed to remove workspace with force: %v", err)
	}
	if report == nil {
		t.Fatalf("expected safety report")
	}

	// Verify that destination directory no longer exists
	if _, err := os.Stat(secondDir); !os.IsNotExist(err) {
		t.Errorf("expected secondDir to be removed, but still exists on disk")
	}

	// Verify that the branch 'feat/test' is PRESERVED in Git
	exists, err := runner.BranchExists(ctx, mainDir, "feat/test")
	if err != nil || !exists {
		t.Errorf("expected branch 'feat/test' to remain preserved in Git history, exists=%v, err=%v", exists, err)
	}
}
