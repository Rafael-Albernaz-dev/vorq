package cli

import (
	"io"
	"strings"
	"testing"
)

func TestMissingGitBlocksOperationsWithoutPanic(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cmd := NewRootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "git binary not found") {
		t.Fatalf("missing Git not reported: %v", err)
	}
	version := NewRootCmd()
	version.SetOut(io.Discard)
	version.SetErr(io.Discard)
	version.SetArgs([]string{"--version"})
	if err := version.Execute(); err != nil {
		t.Fatalf("version unnecessarily requires Git: %v", err)
	}
}
