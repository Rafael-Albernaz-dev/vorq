package secrets

import (
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// Test secrets are assembled at run time so no literal credential is ever
// committed (GitHub push protection and the vorq guard itself would flag it).
func join(parts ...string) string { return strings.Join(parts, "") }

var (
	awsKey    = join("AKIA", "Z7QWERTYUIOPLKJH")
	ghToken   = join("ghp", "_", strings.Repeat("aB3dE5fG7h", 3), "J9kL1m")
	antKey    = join("sk-", "ant-", "api03-", "Qw3rTy7Ui9Op2As4Df6Gh8")
	openaiKey = join("sk-", "proj-", "Zx9Cv8Bn7Mq6Wl5Ek4Rj3Th2Yg1Uf0Id")
	pemHeader = join("-----BEGIN ", "OPENSSH PRIVATE", " KEY-----")
	password  = join("Vq7", "#pL2", "zR9!", "mK4")
)

func rules(fs []domain.GuardFinding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Rule)
	}
	return out
}

func patchFor(path string, added ...string) string {
	var b strings.Builder
	b.WriteString("diff --git a/" + path + " b/" + path + "\nnew file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/" + path + "\n@@ -0,0 +1," + "9 @@\n")
	for _, l := range added {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}

func TestTokenRulesDetectAndMask(t *testing.T) {
	s, _ := NewScanner("")
	cases := map[string]string{
		"aws-access-key": "aws_id = " + awsKey,
		"github-token":   "const t = \"" + ghToken + "\"",
		"anthropic-key":  "key: " + antKey,
		"openai-key":     "OPENAI=" + openaiKey,
		"private-key":    pemHeader,
		"credential-url": "url = postgres://admin:" + password + "@db.internal:5432/app",
		"generic-secret": "DB_PASSWORD=" + password,
	}
	for rule, line := range cases {
		fs := s.ScanContent("config/app.txt", line)
		if rule == "generic-secret" {
			fs = s.ScanContent("config/app.env", line)
		}
		if len(fs) != 1 || fs[0].Rule != rule || !fs[0].Blocking || fs[0].Line != 1 {
			t.Fatalf("%s: got %+v", rule, fs)
		}
		for _, secret := range []string{awsKey, ghToken, antKey, openaiKey, password} {
			if strings.Contains(fs[0].Excerpt, secret) {
				t.Fatalf("%s: excerpt leaks the value: %q", rule, fs[0].Excerpt)
			}
		}
	}
}

func TestGenericRuleIgnoresPlaceholdersAndCode(t *testing.T) {
	s, _ := NewScanner("")
	for _, line := range []string{
		`password = os.Getenv("DB_PASSWORD")`,
		`token := cfg.Token`,
		`PASSWORD=changeme123`,
		`api_key: "${API_KEY}"`,
		`password: "<your-password>"`,
		`token_url = "https://example.org/oauth"`,
		`max_tokens: 4096`,
		`password = getPassword()`,
		`token: ${{ secrets.TOKEN }}`,
		`SECRET_NAME="production-db-credentials"`,
	} {
		path := "app/main.go"
		if strings.Contains(line, "PASSWORD=") || strings.HasPrefix(line, "token:") {
			path = ".env"
		}
		for _, f := range s.ScanContent(path, line) {
			if f.Rule == "generic-secret" {
				t.Fatalf("false positive for %q: %+v", line, f)
			}
		}
	}
	if fs := s.ScanContent("app/main.go", `password := "`+password+`"`); len(fs) != 1 || fs[0].Rule != "generic-secret" {
		t.Fatalf("quoted literal in code must be found: %+v", fs)
	}
}

func TestSensitivePaths(t *testing.T) {
	s, _ := NewScanner("")
	for _, p := range []string{".env", "deploy/.env.production", "certs/server.key", "keys/id_ed25519", "infra/terraform.tfstate", "gcp-service-account.json"} {
		if s.PathFinding(p) == nil {
			t.Fatalf("%s must be sensitive", p)
		}
	}
	for _, p := range []string{".env.example", ".env.sample", "keys/id_ed25519.pub", "src/key.go", "README.md"} {
		if f := s.PathFinding(p); f != nil {
			t.Fatalf("%s must not be sensitive: %+v", p, f)
		}
	}
}

func TestPatchScansOnlyAddedLinesAndNewPaths(t *testing.T) {
	s, _ := NewScanner("")
	patch := "diff --git a/app.go b/app.go\nindex 1..2 100644\n--- a/app.go\n+++ b/app.go\n@@ -10,3 +10,3 @@ func x() {\n context\n-old := \"" + ghToken + "\"\n+fine := 1\n+key := \"" + awsKey + "\"\n" +
		"diff --git a/.env b/.env\ndeleted file mode 100644\nindex 1..0\n--- a/.env\n+++ /dev/null\n@@ -1 +0,0 @@\n-DB_PASSWORD=" + password + "\n"
	fs := s.ScanPatch(patch)
	if len(fs) != 1 || fs[0].Rule != "aws-access-key" || fs[0].Path != "app.go" || fs[0].Line != 12 {
		t.Fatalf("only the added AWS key at app.go:12 must be reported: %+v", fs)
	}
	binary := "diff --git a/store.p12 b/store.p12\nnew file mode 100644\nindex 0000000..1\nBinary files /dev/null and b/store.p12 differ\n"
	if fs := s.ScanPatch(binary); len(fs) != 1 || fs[0].Rule != "sensitive-file" {
		t.Fatalf("binary sensitive file must be reported by path: %+v", fs)
	}
	quoted := "diff --git \"a/dir x/\\303\\251/.env\" \"b/dir x/\\303\\251/.env\"\nnew file mode 100644\n--- /dev/null\n+++ \"b/dir x/\\303\\251/.env\"\n@@ -0,0 +1 @@\n+A=1\n"
	if fs := s.ScanPatch(quoted); len(fs) != 1 || fs[0].Path != "dir x/é/.env" {
		t.Fatalf("quoted path must be decoded: %+v", fs)
	}
}

func TestExceptions(t *testing.T) {
	s, err := NewScanner("# fixtures\ntestdata/** aws-access-key\n.env.local\n* jwt\n")
	if err != nil {
		t.Fatal(err)
	}
	if fs := s.ScanContent("pkg/testdata/deep/x.txt", awsKey); len(fs) != 1 {
		t.Fatalf("testdata/** is anchored at the root: %+v", fs)
	}
	if fs := s.ScanContent("testdata/deep/x.txt", awsKey); len(fs) != 0 {
		t.Fatalf("allowlisted rule must be suppressed: %+v", fs)
	}
	if fs := s.ScanContent("testdata/x.txt", ghToken); len(fs) != 1 {
		t.Fatalf("other rules still apply on allowlisted paths: %+v", fs)
	}
	if s.PathFinding("app/.env.local") != nil {
		t.Fatal("base-name glob must allow the file at any depth")
	}
	if fs := s.ScanContent("a.txt", "x := \""+ghToken+"\" // "+InlineAllow); len(fs) != 0 {
		t.Fatalf("inline exception must suppress the line: %+v", fs)
	}
	if _, err := NewScanner("[bad"); err == nil {
		t.Fatal("invalid glob must be rejected")
	}
}

func TestConflictDebugLargeAndCase(t *testing.T) {
	s, _ := NewScanner("")
	fs := s.ScanPatch(patchFor("main.go", "<<<<<<< HEAD", "a := 1", "=======", "a := 2", ">>>>>>> feature", `fmt.Println("DEBUG x")`))
	got := strings.Join(rules(fs), ",")
	if got != "debug-statement,conflict-marker" {
		t.Fatalf("got %s (%+v)", got, fs)
	}
	for _, f := range fs {
		if f.Rule == "debug-statement" && f.Blocking || f.Rule == "conflict-marker" && !f.Blocking {
			t.Fatalf("wrong severity: %+v", f)
		}
	}
	if fs := s.ScanPatch(patchFor("README.md", "Title", "=======")); len(fs) != 0 {
		t.Fatalf("markdown underline is not a conflict: %+v", fs)
	}
	if s.LargeFile("big.bin", LargeFileLimit) != nil || s.LargeFile("big.bin", LargeFileLimit+1) == nil {
		t.Fatal("large file threshold")
	}
	cc := s.CaseConflicts([]string{"Readme.md", "README.md", "a/X.go", "a/x.go"}, []string{"README.md"})
	if len(cc) != 1 || cc[0].Blocking || !strings.Contains(cc[0].Message, "Readme.md") {
		t.Fatalf("case conflict: %+v", cc)
	}
}

func TestMask(t *testing.T) {
	if Mask("short") != "****" || Mask(awsKey) != "AKIA…LKJH" {
		t.Fatalf("mask: %q %q", Mask("short"), Mask(awsKey))
	}
}
