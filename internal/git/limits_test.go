package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecOutputHelper(t *testing.T) {
	if os.Getenv("VORQ_TEST_GIT_OUTPUT") != "1" {
		return
	}
	chunk := strings.Repeat("x", 64*1024)
	for i := 0; i < MaxCommandOutput/len(chunk)+2; i++ {
		fmt.Print(chunk)
	}
	os.Exit(0)
}
func TestExecRejectsOversizedOutputInsteadOfParsingPrefix(t *testing.T) {
	t.Setenv("VORQ_TEST_GIT_OUTPUT", "1")
	r := &NativeRunner{gitBinPath: os.Args[0]}
	result, err := r.Exec(context.Background(), t.TempDir(), "-test.run=^TestExecOutputHelper$")
	var limit *OutputLimitError
	if result != "" || !errors.As(err, &limit) || limit.Limit != MaxCommandOutput {
		t.Fatalf("oversized output accepted: len=%d %v", len(result), err)
	}
}

func TestSuccessfulCommitWithOversizedOutputRetainsCreatedOutcome(t *testing.T) {
	t.Setenv("VORQ_TEST_GIT_OUTPUT", "1")
	dir := t.TempDir()
	helper := "'" + strings.ReplaceAll(os.Args[0], "'", "'\"'\"'") + "'"
	hash := strings.Repeat("a", 40)
	// This executable simulates the Git process, including a successful exit
	// after large commit output. No real repository or commit is modified.
	script := "#!/bin/sh\nif [ \"$1\" = commit ]; then\nexec " + helper + " -test.run=^TestExecOutputHelper$\nfi\nprintf '%s\\n' '" + hash + "'\n"
	path := filepath.Join(dir, "git-output-fixture")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	actual, err := (&NativeRunner{gitBinPath: path}).Commit(context.Background(), dir, "test")
	if err != nil || actual != hash {
		t.Fatalf("successful commit became retryable: %q %v", actual, err)
	}
}

func TestMalformedStatusNeverBecomesClean(t *testing.T) {
	for _, data := range []string{"M", "M\x00", " M file", "?? \x00", "XX file\x00", "R  file\x00", "?M file\x00", "\x00", "   file\x00"} {
		if _, err := ParseStatus(data, "/workspace", "main"); err == nil {
			t.Errorf("invalid record accepted: %q", data)
		}
	}
}
