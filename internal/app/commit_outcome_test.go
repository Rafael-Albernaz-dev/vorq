package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/app"
	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
	"github.com/Rafael-Albernaz-dev/vorq/internal/git"
)

type metadataFailureRunner struct {
	git.Runner
	mode    string
	commits int
	cause   error
}

func (r *metadataFailureRunner) Exec(ctx context.Context, dir string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "log" {
		if r.mode == "subject" {
			return "", r.cause
		}
		if r.mode == "malformed" {
			return "unexpected metadata", nil
		}
	}
	if len(args) > 0 && args[0] == "diff-tree" && r.mode == "files" {
		return "", r.cause
	}
	return r.Runner.Exec(ctx, dir, args...)
}
func (r *metadataFailureRunner) Commit(ctx context.Context, dir, message string) (string, error) {
	r.commits++
	hash, err := r.Runner.Commit(ctx, dir, message)
	if err == nil && r.mode == "head" {
		return "", &domain.CommitMetadataError{Operation: "resolving HEAD", Err: r.cause}
	}
	return hash, err
}

func TestApp_Commit_MetadataFailurePreservesCreatedResult(t *testing.T) {
	for _, mode := range []string{"subject", "malformed", "files"} {
		t.Run(mode, func(t *testing.T) {
			original, native, dir := setupCommitApp(t)
			ctx := context.Background()
			writeCommitFile(t, dir, "real.txt", "real contents\n")
			if err := original.StageFiles(ctx, dir, []string{"real.txt"}); err != nil {
				t.Fatal(err)
			}
			hooks := t.TempDir()
			if _, err := native.Exec(ctx, dir, "config", "core.hooksPath", hooks); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(hooks, "commit-msg"), []byte("#!/bin/sh\nprintf 'fix: actual hook subject\\n' > \"$1\"\n"), 0755); err != nil {
				t.Fatal(err)
			}
			cause := errors.New("metadata read unavailable")
			runner := &metadataFailureRunner{Runner: native, mode: mode, cause: cause}
			result, err := app.NewApp(runner).CreateCommit(ctx, dir, "feat: proposed subject")
			var outcome *domain.CommitMetadataError
			if err == nil || !errors.As(err, &outcome) {
				t.Fatalf("expected typed post-commit failure, got %+v / %v", result, err)
			}
			head, headErr := native.RevParseHead(ctx, dir)
			if headErr != nil || result == nil || result.Hash != head || result.ShortHash != head[:7] || outcome.Hash != head {
				t.Fatalf("lost confirmed commit identity: %+v %v, head %s %v", result, err, head, headErr)
			}
			if mode == "files" && result.Subject != "fix: actual hook subject" {
				t.Fatalf("lost actual hook subject: %+v", result)
			}
			if mode != "files" && result.Subject != "" {
				t.Fatalf("invented subject from proposed message: %+v", result)
			}
			if mode != "malformed" && !errors.Is(err, cause) {
				t.Fatalf("cause not wrapped: %v", err)
			}
			count, countErr := native.Exec(ctx, dir, "rev-list", "--count", "HEAD")
			if countErr != nil || strings.TrimSpace(count) != "1" || runner.commits != 1 {
				t.Fatalf("commit was repeated: count %q calls %d err %v", count, runner.commits, countErr)
			}
			status, statusErr := native.Status(ctx, dir)
			if statusErr != nil || status != "" {
				t.Fatalf("commit was not created: %q %v", status, statusErr)
			}
		})
	}
}

func TestApp_Commit_HookFailureHasNoCreatedResult(t *testing.T) {
	original, native, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "real.txt", "real contents\n")
	if err := original.StageFiles(ctx, dir, []string{"real.txt"}); err != nil {
		t.Fatal(err)
	}
	hooks := t.TempDir()
	if _, err := native.Exec(ctx, dir, "config", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho 'hook rejected' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	result, err := original.CreateCommit(ctx, dir, "feat: rejected")
	var metadataErr *domain.CommitMetadataError
	if err == nil || result != nil || errors.As(err, &metadataErr) {
		t.Fatalf("hook failure reported a created commit: %+v %v", result, err)
	}
	if _, err := native.RevParseHead(ctx, dir); err == nil {
		t.Fatal("hook rejection still created commit")
	}
}

func TestApp_Commit_PropagatesCreatedOutcomeWithoutHash(t *testing.T) {
	original, native, dir := setupCommitApp(t)
	ctx := context.Background()
	writeCommitFile(t, dir, "real.txt", "real contents\n")
	if err := original.StageFiles(ctx, dir, []string{"real.txt"}); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("HEAD unavailable")
	runner := &metadataFailureRunner{Runner: native, mode: "head", cause: cause}
	result, err := app.NewApp(runner).CreateCommit(ctx, dir, "feat: already created")
	var outcome *domain.CommitMetadataError
	if result != nil || !errors.As(err, &outcome) || !errors.Is(err, cause) || outcome.Hash != "" {
		t.Fatalf("lost unknown-hash created outcome: %+v %v", result, err)
	}
	count, countErr := native.Exec(ctx, dir, "rev-list", "--count", "HEAD")
	if countErr != nil || strings.TrimSpace(count) != "1" || runner.commits != 1 {
		t.Fatalf("commit repeated: %q calls %d %v", count, runner.commits, countErr)
	}
}
