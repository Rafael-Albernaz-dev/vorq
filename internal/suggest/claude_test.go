package suggest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// fakeClient installs an executable named name in PATH that records its
// arguments and environment, then runs body.
func fakeClient(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "record")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + record + ".args\nenv > " + record + ".env\ncat > " + record + ".stdin\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return record
}

func TestClaudeSubscriptionIsolationAndStructuredOutput(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "must-not-leak")
	t.Setenv("CLAUDECODE", "1")
	envelope := `{"type":"result","subtype":"success","is_error":false,"result":"ignored","structured_output":{"subject":"feat: add greeting","body":"Print a greeting."}}`
	record := fakeClient(t, "claude", "printf '%s' '"+envelope+"'")
	p, err := NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "claude", Model: "sonnet", Diff: "+hi"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "feat: add greeting" || p.Body != "Print a greeting." || p.ModelName != "sonnet" {
		t.Fatalf("unexpected proposal %+v", p)
	}
	args, _ := os.ReadFile(record + ".args")
	lines := strings.Split(string(args), "\n")
	joined := strings.Join(lines, " ")
	for _, want := range []string{"-p", "--output-format json", "--strict-mcp-config", "--disable-slash-commands", "--no-session-persistence", "--json-schema", "--model sonnet"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	for i, arg := range lines {
		if arg == "--tools" && (i+1 >= len(lines) || lines[i+1] != "") {
			t.Error(`--tools must be followed by "" (no tools)`)
		}
		if arg == "--bare" {
			t.Error("--bare disables subscription login and must not be used")
		}
	}
	env, _ := os.ReadFile(record + ".env")
	if strings.Contains(string(env), "ANTHROPIC_API_KEY") || strings.Contains(string(env), "CLAUDECODE") {
		t.Fatal("API key or parent Claude session leaked into the client environment")
	}
	stdin, _ := os.ReadFile(record + ".stdin")
	if !strings.Contains(string(stdin), `"+hi"`) {
		t.Fatal("prompt with the diff was not sent on stdin")
	}
}

func TestClaudeErrorsCarryTheClientReason(t *testing.T) {
	fakeClient(t, "claude", `printf '%s' '{"type":"result","subtype":"success","is_error":true,"result":"Not logged in · Please run /login"}'; exit 1`)
	_, err := NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "claude", Diff: "+x"})
	if err == nil || !strings.Contains(err.Error(), "Not logged in") {
		t.Fatalf("error must include the client reason, got %v", err)
	}
}

func TestProviderStderrIsSummarisedInErrors(t *testing.T) {
	fakeClient(t, "codex", `printf 'line one\nERROR: \033[31mquota exceeded\033[0m for plan\n' >&2; exit 2`)
	_, err := NewCLIProvider().Suggest(context.Background(), domain.AIRequest{Provider: "codex", Diff: "+x"})
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("stderr detail missing: %v", err)
	}
	if strings.ContainsRune(err.Error(), '\x1b') || strings.Contains(err.Error(), "[31m") {
		t.Fatal("control characters leaked into the error")
	}
	if detail := providerDetail(strings.Repeat("x", 2000)); len([]rune(detail)) > 300 {
		t.Fatalf("detail not bounded: %d", len([]rune(detail)))
	}
}

func TestClaudeModelCatalogOffersAliases(t *testing.T) {
	fakeClient(t, "claude", "true")
	models, err := NewCLIProvider().ListModels(context.Background(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != ",fable,opus,sonnet,haiku" {
		t.Fatalf("catalog %v", ids)
	}
}

func TestClaudePlanSchemaRestrictsFileIDs(t *testing.T) {
	envelope := `{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":{"commits":[]}}`
	record := fakeClient(t, "claude", "printf '%s' '"+envelope+"'")
	payload := `[{"id":"F0001","path":"a.go","old_path":"","diff":"+x"},{"id":"F0002","path":"b.go","old_path":"","diff":"+y"}]`
	if _, err := NewCLIProvider().Analyze(context.Background(), domain.AnalysisRequest{Provider: "claude", Task: "plan", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(record + ".args")
	if !strings.Contains(string(args), `"enum":["F0001","F0002"]`) {
		t.Fatalf("schema must enumerate supplied IDs:\n%s", args)
	}
	if strings.Contains(string(args), "__VORQ_FILE_ID__") {
		t.Fatal("placeholder leaked into the schema")
	}
}
