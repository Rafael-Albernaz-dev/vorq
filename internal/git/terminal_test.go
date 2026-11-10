package git_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// VORQ-012: in the Studio (no terminal), an HTTPS remote asking for
// credentials fails fast with a typed error instead of prompting on the TTY.
func TestPushWithoutTerminalFailsFastWithTypedError(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	ctx := context.Background()
	runner, dir := setupRunnerRepo(t)
	writeRunnerFile(t, dir, "a.txt", "a\n")
	for _, args := range [][]string{{"add", "a.txt"}, {"commit", "-m", "init"}, {"remote", "add", "origin", server.URL + "/repo.git"}} {
		if _, err := runner.Exec(ctx, dir, args...); err != nil {
			t.Fatal(args, err)
		}
	}
	start := time.Now()
	_, err := runner.Push(domain.WithoutTerminal(ctx), dir, "origin", "main", "main", true)
	var needs *domain.TerminalRequiredError
	if !errors.As(err, &needs) {
		t.Fatalf("want TerminalRequiredError, got %v", err)
	}
	if time.Since(start) > 20*time.Second {
		t.Fatal("push did not fail fast")
	}
	// The CLI (terminal allowed) keeps the plain native error semantics.
	// GIT_TERMINAL_PROMPT=0 keeps this test from prompting when run in a TTY.
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	if _, err := runner.Push(ctx, dir, "origin", "main", "main", true); errors.As(err, &needs) {
		t.Fatal("terminal-allowed context must not be classified as non-interactive")
	}
}

func TestNonInteractiveCommandsHaveNoControllingTerminal(t *testing.T) {
	runner, dir := setupRunnerRepo(t)
	out, err := runner.Exec(domain.WithoutTerminal(context.Background()), dir, "-c", "alias.probe=!(exec 3</dev/tty) 2>/dev/null && echo TTY || echo NOTTY", "probe")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "NOTTY" {
		t.Fatalf("non-interactive git could open the TTY: %q", out)
	}
}
