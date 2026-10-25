package suggest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// A subprocess fixture exercises stdin, argv, environment and output parsing.
// It never contacts a provider or reads subscription credentials.
func TestCLISubprocess(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	script := `#!/bin/sh
case "$*" in *--ignore-user-config*--ephemeral*read-only*--model*gpt-test*) ;; *) exit 7;; esac
[ -z "$OPENAI_API_KEY" ] || exit 8
[ -z "$GEMINI_API_KEY" ] || exit 8
[ ! -e ./private.txt ] || exit 9
input=$(cat)
case "$input" in *"staged fixture"*) ;; *) exit 10;; esac
printf '%s' '{"subject":"fix: handle staged changes","body":"Preserve the reviewed content."}'
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("OPENAI_API_KEY", "test-key-must-not-reach-child")
	t.Setenv("GEMINI_API_KEY", "test-key-must-not-reach-child")
	p, err := NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "codex", Model: "gpt-test", Diff: "staged fixture"})
	if err != nil || p.Subject != "fix: handle staged changes" {
		t.Fatalf("result %+v %v", p, err)
	}
	if err = os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s' 'not json'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "codex"}); err == nil {
		t.Fatal("malformed output accepted")
	}
	if err = os.WriteFile(bin, []byte("#!/bin/sh\nsleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = NewCLIProvider().Suggest(ctx, domain.AIRequest{Provider: "codex"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wrong cancellation: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("child cancellation too slow")
	}
}

func TestSubscriptionEnvironment(t *testing.T) {
	env := subscriptionEnv([]string{"PATH=/bin", "HOME=/safe", "OPENAI_API_KEY=secret", "GEMINI_API_KEY=secret", "CODEX_API_KEY=secret", "GOOGLE_GENAI_USE_VERTEXAI=true", "GOOGLE_APPLICATION_CREDENTIALS=/private", "ANTIGRAVITY_PROJECT_ID=private"})
	if strings.Join(env, ";") != "PATH=/bin;HOME=/safe" {
		t.Fatalf("unexpected environment: %v", env)
	}
}

func TestGeminiSubprocessIsolationAndEvents(t *testing.T) {
	home, binDir := t.TempDir(), t.TempDir()
	config := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(filepath.Join(config, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "antigravity-oauth-token"), []byte("fixture-not-a-token"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
	bin := filepath.Join(binDir, "agy")
	script := `#!/bin/sh
[ -L "$HOME/.gemini/antigravity-cli/antigravity-oauth-token" ] || exit 10
[ -f "$HOME/.gemini/antigravity-cli/settings.json" ] || exit 11
[ ! -e "$HOME/.gemini/config/hooks.json" ] || exit 12
[ -f .agents/agents/vorq-message.md ] || exit 13
case "$*" in *--input-format*stream-json*--disable-slash-commands*--model*gemini-3.8-flash-low*) ;; *) exit 14;; esac
cat >/dev/null
printf '%s\n' '{"event":"init","init":{"tools":[]}}' '{"event":"result","result":{"status":"SUCCESS","response":"{\"subject\":\"docs: clarify usage\",\"body\":\"Explain the staged documentation change.\"}"}}'
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	p, err := NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "gemini", Diff: "fixture"})
	if err != nil || p.Subject != "docs: clarify usage" || p.ModelName != "gemini-3.8-flash-low" {
		t.Fatalf("Gemini result: %+v %v", p, err)
	}
	for _, output := range []string{
		`{"event":"result","result":{"status":"ERROR"}}`,
		`{"event":"result","result":{"status":"SUCCESS","response":"{\"subject\":\"docs: usage\"}"}}`,
		`{"event":"step_update","step_update":{"step_type":"tool_call"}}`,
		`{"event":"result","result":{"status":"SUCCESS","response":"{\"subject\":\"docs: usage\",\"body\":\"text\"}"}}
{"event":"result","result":{"status":"SUCCESS"}}`,
	} {
		// The fixture is JSON controlled by this test and has no shell quotes.
		if err := os.WriteFile(bin, []byte("#!/bin/sh\ncat >/dev/null\nprintf '%s' '"+output+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "gemini"}); err == nil {
			t.Fatalf("invalid event accepted: %s", output)
		}
	}
}

// Explicitly opt in: these checks use the account's actual subscription quota.
func TestLiveAIProvider(t *testing.T) {
	provider := os.Getenv("VORQ_LIVE_AI")
	if provider == "" {
		t.Skip("set VORQ_LIVE_AI=codex or gemini to exercise a real provider")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	p, err := NewCLIProvider().Suggest(ctx, domain.AIRequest{Provider: provider, Diff: "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-Run vorq to start.\n+Run vorq up to open the Workspace Studio.\n"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Subject, "docs") || p.Body == "" {
		t.Fatalf("unexpected real suggestion: %+v", p)
	}
	t.Logf("%s real response: %s\n%s", provider, p.Subject, p.Body)
}
