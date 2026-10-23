package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

func setupRunnerRepo(t *testing.T) (*git.NativeRunner, string) {
	t.Helper()
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", filepath.Join(dir, "hooks")},
	} {
		if _, err := runner.Exec(context.Background(), dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return runner, dir
}

func writeRunnerFile(t *testing.T, dir, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNativeRunner_Add(t *testing.T) {
	runner, dir := setupRunnerRepo(t)
	ctx := context.Background()
	for _, path := range []string{"espaço ação.txt", "[literal].txt", "l.txt", "--option"} {
		writeRunnerFile(t, dir, path, "content")
	}
	for _, path := range []string{"espaço ação.txt", "[literal].txt", "--option"} {
		if err := runner.Add(ctx, dir, path); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := runner.Status(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "A  espaço ação.txt\x00") || !strings.Contains(raw, "?? l.txt\x00") || !strings.Contains(raw, "A  [literal].txt\x00") {
		t.Fatalf("literal stage failed: %q", raw)
	}
	for _, sentinel := range []string{".", "-A"} {
		if err := runner.Add(ctx, dir, sentinel); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Add(ctx, dir); err == nil {
		t.Fatal("accepted empty paths")
	}
	if err := runner.Add(ctx, dir, "", "l.txt"); err == nil {
		t.Fatal("accepted empty path")
	}
	if err := runner.Add(ctx, dir, ".", "l.txt"); err == nil {
		t.Fatal("accepted ambiguous all")
	}
}

func TestNativeRunner_Reset(t *testing.T) {
	runner, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "ação.txt", "data")
	if err := runner.Add(ctx, dir, "ação.txt"); err != nil {
		t.Fatal(err)
	}
	// Unstage is also supported before the initial commit.
	if err := runner.Reset(ctx, dir, "ação.txt"); err != nil {
		t.Fatal(err)
	}
	raw, err := runner.Status(ctx, dir)
	if err != nil || !strings.Contains(raw, "?? ação.txt\x00") {
		t.Fatalf("status %q: %v", raw, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "ação.txt")); err != nil || string(data) != "data" {
		t.Fatal("working file changed")
	}
	if err := runner.Reset(ctx, dir); err == nil {
		t.Fatal("accepted empty paths")
	}
}

func TestNativeRunner_Commit_And_RevParseHead(t *testing.T) {
	runner, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "file.txt", "first\n")
	if err := runner.Add(ctx, dir, "file.txt"); err != nil {
		t.Fatal(err)
	}
	// The content after stage must not leak into the created commit.
	writeRunnerFile(t, dir, "file.txt", "second\n")
	hash, err := runner.Commit(ctx, dir, "feat: first 'message' $(literal)")
	if err != nil {
		t.Fatal(err)
	}
	logged, err := runner.Exec(ctx, dir, "log", "-1", "--format=%H")
	if err != nil || strings.TrimSpace(logged) != hash {
		t.Fatalf("hash %q != log %q: %v", hash, logged, err)
	}
	head, err := runner.RevParseHead(ctx, dir)
	if err != nil || head != hash {
		t.Fatalf("HEAD %q: %v", head, err)
	}
	content, err := runner.Exec(ctx, dir, "show", "HEAD:file.txt")
	if err != nil || content != "first\n" {
		t.Fatalf("content %q: %v", content, err)
	}
	if _, err := runner.Commit(ctx, dir, "  "); err == nil {
		t.Fatal("accepted blank message")
	}
}

func TestNativeRunner_Commit_PreservesHooks(t *testing.T) {
	runner, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "file.txt", "data")
	if err := runner.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(dir, "hooks")
	if err := os.Mkdir(hooks, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho blocked-by-test-hook >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Commit(ctx, dir, "feat: hook check"); err == nil || !strings.Contains(err.Error(), "blocked-by-test-hook") {
		t.Fatalf("hook not honored: %v", err)
	}
	if _, err := runner.RevParseHead(ctx, dir); err == nil {
		t.Fatal("commit created despite hook")
	}
}

func TestNativeRunner_Commit_HonorsSigning(t *testing.T) {
	runner, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "file.txt", "data")
	if err := runner.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"config", "commit.gpgsign", "true"},
		{"config", "gpg.format", "openpgp"},
		{"config", "gpg.program", filepath.Join(dir, "missing-test-gpg")},
	} {
		if _, err := runner.Exec(ctx, dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runner.Commit(ctx, dir, "feat: signing check"); err == nil || !strings.Contains(err.Error(), "missing-test-gpg") {
		t.Fatalf("configured signer was bypassed: %v", err)
	}
	if _, err := runner.RevParseHead(ctx, dir); err == nil {
		t.Fatal("commit created without required signing")
	}
}

func TestNativeRunner_Version(t *testing.T) {
	runner, err := git.NewNativeRunner()
	if err != nil {
		t.Fatalf("failed to initialize NativeRunner: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ver, err := runner.Version(ctx)
	if err != nil {
		t.Fatalf("expected git --version to succeed, got error: %v", err)
	}

	if !strings.HasPrefix(ver, "git version") {
		t.Errorf("expected version output to start with 'git version', got %q", ver)
	}
}

func TestNativeRunner_PushAndUpstream(t *testing.T) {
	r, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "file.txt", "initial\n")
	if err := r.Add(ctx, dir, "."); err != nil {
		t.Fatal(err)
	}
	hash, err := r.Commit(ctx, dir, "chore: initial")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Upstream(ctx, dir); err == nil {
		t.Fatal("unexpected upstream")
	}
	remote := t.TempDir()
	if _, err := r.Exec(ctx, remote, "init", "--bare"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Exec(ctx, dir, "remote", "add", "origin", remote); err != nil {
		t.Fatal(err)
	}
	remotes, err := r.Remotes(ctx, dir)
	if err != nil || len(remotes) != 1 || remotes[0] != "origin" {
		t.Fatalf("remotes %v: %v", remotes, err)
	}
	branch, err := r.CurrentBranch(ctx, dir)
	if err != nil || branch != "main" {
		t.Fatalf("branch %q: %v", branch, err)
	}
	if _, err := r.Push(ctx, dir, "origin", "main", "release", true); err != nil {
		t.Fatal(err)
	}
	upstream, err := r.Upstream(ctx, dir)
	if err != nil || upstream != "origin/release" {
		t.Fatalf("upstream %q: %v", upstream, err)
	}
	out, err := r.Exec(ctx, remote, "rev-parse", "refs/heads/release")
	if err != nil || strings.TrimSpace(out) != hash {
		t.Fatalf("remote hash %q: %v", out, err)
	}
}

func TestNativeRunner_FileDiff_Limit(t *testing.T) {
	r, dir := setupRunnerRepo(t)
	ctx := context.Background()
	writeRunnerFile(t, dir, "[literal].txt", strings.Repeat("line of content\n", 50000))
	if err := r.Add(ctx, dir, "[literal].txt"); err != nil {
		t.Fatal(err)
	}
	diff, truncated, err := r.FileDiff(ctx, dir, "[literal].txt", true)
	if err != nil || !truncated || len(diff) > 256*1024 {
		t.Fatalf("diff limit len=%d truncated=%v: %v", len(diff), truncated, err)
	}
}
