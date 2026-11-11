// Package secrets is the local guardrail engine (VORQ-018): built-in rules
// inspired by gitleaks (MIT), pure Go, no network and no external tools.
// It reports masked findings; it never stores or returns a detected value.
package secrets

import (
	"math"
	"path"
	"regexp"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// LargeFileLimit is the size above which an added file is reported.
const LargeFileLimit = 5 * 1024 * 1024

// InlineAllow on a line suppresses content findings for that line.
const InlineAllow = "vorq:allow-secret"

type tokenRule struct {
	id, message string
	re          *regexp.Regexp
	group       int // submatch holding the secret (0 = whole match)
}

// Ordered: the first matching rule reports the line (sk-ant before sk-).
var tokenRules = []tokenRule{
	{"private-key", "private key block", regexp.MustCompile(`-----BEGIN[ A-Z0-9_-]*PRIVATE KEY( BLOCK)?-----`), 0},
	{"aws-access-key", "AWS access key ID", regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}\b`), 0},
	{"github-token", "GitHub token", regexp.MustCompile(`\b(?:(?:ghp|gho|ghu|ghs|ghr)_[0-9A-Za-z]{36}|github_pat_[0-9A-Za-z_]{40,})\b`), 0},
	{"gitlab-token", "GitLab token", regexp.MustCompile(`\bglpat-[0-9A-Za-z_-]{20,}`), 0},
	{"slack-webhook", "Slack webhook URL", regexp.MustCompile(`https://hooks\.slack\.com/services/T[0-9A-Za-z]+/B[0-9A-Za-z]+/[0-9A-Za-z]+`), 0},
	{"slack-token", "Slack token", regexp.MustCompile(`\bxox[abprs]-[0-9A-Za-z-]{10,}`), 0},
	{"stripe-key", "Stripe live key", regexp.MustCompile(`\b(?:sk|rk)_live_[0-9A-Za-z]{20,}`), 0},
	{"google-api-key", "Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`), 0},
	{"anthropic-key", "Anthropic API key", regexp.MustCompile(`\bsk-ant-[0-9A-Za-z_-]{20,}`), 0},
	{"openai-key", "OpenAI API key", regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[0-9A-Za-z_-]{32,}`), 0},
	{"npm-token", "npm token", regexp.MustCompile(`\bnpm_[0-9A-Za-z]{36}\b`), 0},
	{"npmrc-auth", "registry auth token", regexp.MustCompile(`(?i)_auth(?:Token)?\s*=\s*([^\s$"'{}]{8,})`), 1},
	{"sendgrid-key", "SendGrid API key", regexp.MustCompile(`\bSG\.[0-9A-Za-z_-]{22}\.[0-9A-Za-z_-]{43}\b`), 0},
	{"twilio-key", "Twilio API key", regexp.MustCompile(`\bSK[0-9a-f]{32}\b`), 0},
	{"jwt", "JSON Web Token", regexp.MustCompile(`\beyJ[0-9A-Za-z_-]{10,}\.eyJ[0-9A-Za-z_-]{10,}\.[0-9A-Za-z_-]{10,}`), 0},
}

var credentialURL = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s:/@'"]+:([^\s/@'"]+)@[^\s/'"]+`)

var genericAssignment = regexp.MustCompile(`(?i)([a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|auth[_-]?key|private[_-]?key)[a-z0-9_.-]*)["']?\s*(?::=|=>|=|:)\s*(?:"([^"\s]{8,})"|'([^'\s]{8,})'|([^\s"'` + "`" + `;,]{8,}))`)

// Key suffixes that name metadata about a secret, not the secret itself.
var metadataKey = regexp.MustCompile(`(?i)(?:_|-|\.)?(?:name|type|url|uri|endpoint|path|file|env|header|field|length|len|count|hash|policy|regex|pattern|prompt|dir|ttl|expiry|expires|limit|key_id|id)$`)

var reference = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

var placeholderWords = []string{
	"changeme", "change_me", "change-me", "example", "sample", "placeholder", "dummy", "fake", "redacted",
	"your_", "your-", "yourpass", "xxxx", "****", "....", "todo", "replace", "insert", "<", ">", "${", "{{", "%(", "$(",
}

var conflictStart = regexp.MustCompile(`^<{7}(?: |$)`)
var conflictEnd = regexp.MustCompile(`^>{7}(?: |$)`)

type debugRule struct {
	exts []string
	re   *regexp.Regexp
	what string
}

var debugRules = []debugRule{
	{[]string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".vue", ".svelte"}, regexp.MustCompile(`^\s*debugger\s*;?\s*$`), "debugger statement"},
	{[]string{".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".vue", ".svelte"}, regexp.MustCompile(`\bconsole\.log\s*\(`), "console.log"},
	{[]string{".py"}, regexp.MustCompile(`\b(?:pdb|ipdb|pudb)\.set_trace\s*\(|^\s*breakpoint\s*\(\s*\)`), "Python breakpoint"},
	{[]string{".rb"}, regexp.MustCompile(`\bbinding\.pry\b|^\s*byebug\b`), "Ruby breakpoint"},
	{[]string{".go"}, regexp.MustCompile(`fmt\.Print(?:ln|f)?\(\s*"DEBUG`), "debug print"},
	{[]string{".php"}, regexp.MustCompile(`\b(?:var_dump|dd)\s*\(`), "debug dump"},
}

// sensitivePath reports why a path is a secret-bearing file, or "".
func sensitivePath(p string) string {
	base := strings.ToLower(path.Base(p))
	ext := path.Ext(base)
	switch {
	case base == ".env":
		return "environment file"
	case strings.HasPrefix(base, ".env."):
		for _, safe := range []string{".example", ".sample", ".template", ".dist", ".defaults"} {
			if strings.HasSuffix(base, safe) {
				return ""
			}
		}
		return "environment file"
	case ext == ".pem" || ext == ".key" || ext == ".p12" || ext == ".pfx" || ext == ".jks" || ext == ".keystore":
		return "key or certificate store"
	case base == "id_rsa" || base == "id_dsa" || base == "id_ecdsa" || base == "id_ed25519":
		return "SSH private key"
	case base == ".netrc" || base == "_netrc" || base == ".git-credentials":
		return "credential store"
	case base == "credentials.json" || (strings.Contains(base, "service-account") && ext == ".json"):
		return "cloud credentials"
	case ext == ".tfstate" || strings.HasSuffix(base, ".tfstate.backup"):
		return "Terraform state"
	case base == "kubeconfig":
		return "Kubernetes credentials"
	}
	return ""
}

func configLike(p string) bool {
	base := strings.ToLower(path.Base(p))
	if strings.HasPrefix(base, ".env") || base == ".npmrc" || base == ".pypirc" {
		return true
	}
	switch path.Ext(base) {
	case ".env", ".ini", ".cfg", ".conf", ".properties", ".yaml", ".yml", ".toml":
		return true
	}
	return false
}

func entropy(s string) float64 {
	counts := map[rune]int{}
	n := 0
	for _, r := range s {
		counts[r]++
		n++
	}
	var h float64
	for _, c := range counts {
		p := float64(c) / float64(n)
		h -= p * math.Log2(p)
	}
	return h
}

func placeholder(value string) bool {
	lower := strings.ToLower(value)
	for _, w := range placeholderWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	for _, w := range []string{"password", "passwd", "secret", "token", "null", "none", "true", "false"} {
		if lower == w {
			return true
		}
	}
	return strings.Trim(lower, "x*.-_0") == ""
}

// genericValue decides whether an assignment's value looks like a real secret.
func genericValue(p, key, value string, quoted bool) bool {
	if metadataKey.MatchString(key) || placeholder(value) {
		return false
	}
	if strings.HasPrefix(value, "$") || strings.ContainsAny(value, "()[]") || strings.HasPrefix(strings.ToLower(value), "http") {
		return false
	}
	if !quoted {
		if !configLike(p) {
			return false // in code, an unquoted value is a variable/expression
		}
		if strings.Contains(value, ".") && reference.MatchString(value) {
			return false // references such as secrets.TOKEN or env.X
		}
	}
	return entropy(value) >= 3.0
}

// Mask shows only the edges of a value.
func Mask(value string) string {
	r := []rune(value)
	switch {
	case len(r) <= 8:
		return "****"
	case len(r) <= 16:
		return string(r[:2]) + "…" + string(r[len(r)-2:])
	}
	return string(r[:4]) + "…" + string(r[len(r)-4:])
}

func excerpt(line string, start, end int) string {
	masked := line[:start] + Mask(line[start:end]) + line[end:]
	masked = strings.TrimSpace(masked)
	if r := []rune(masked); len(r) > 120 {
		masked = string(r[:117]) + "…"
	}
	return masked
}

func finding(rule, category string, blocking bool, p string, line int, ex, message string) domain.GuardFinding {
	return domain.GuardFinding{Rule: rule, Category: category, Blocking: blocking, Path: p, Line: line, Excerpt: ex, Message: message}
}

// scanSecretLine returns at most one secret finding for one added line.
func scanSecretLine(p, line string, n int) *domain.GuardFinding {
	if strings.Contains(line, InlineAllow) {
		return nil
	}
	for _, rule := range tokenRules {
		loc := rule.re.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}
		s, e := loc[2*rule.group], loc[2*rule.group+1]
		if rule.id == "npmrc-auth" && placeholder(line[s:e]) {
			continue
		}
		f := finding(rule.id, domain.GuardSecret, true, p, n, excerpt(line, s, e), rule.message)
		return &f
	}
	for _, loc := range credentialURL.FindAllStringSubmatchIndex(line, -1) {
		if pass := line[loc[2]:loc[3]]; !placeholder(pass) && !strings.HasPrefix(pass, "$") {
			f := finding("credential-url", domain.GuardSecret, true, p, n, excerpt(line, loc[2], loc[3]), "URL with embedded password")
			return &f
		}
	}
	for _, loc := range genericAssignment.FindAllStringSubmatchIndex(line, -1) {
		key := line[loc[2]:loc[3]]
		for g, quoted := range map[int]bool{2: true, 3: true, 4: false} {
			if loc[2*g] < 0 {
				continue
			}
			value := line[loc[2*g]:loc[2*g+1]]
			if genericValue(p, key, value, quoted) {
				f := finding("generic-secret", domain.GuardSecret, true, p, n, excerpt(line, loc[2*g], loc[2*g+1]), "hard-coded password, secret or token")
				return &f
			}
		}
	}
	return nil
}

func debugFinding(p, line string, n int) *domain.GuardFinding {
	ext := strings.ToLower(path.Ext(p))
	for _, rule := range debugRules {
		for _, e := range rule.exts {
			if e == ext && rule.re.MatchString(line) {
				ex := strings.TrimSpace(line)
				if r := []rune(ex); len(r) > 120 {
					ex = string(r[:117]) + "…"
				}
				f := finding("debug-statement", domain.GuardDebug, false, p, n, ex, rule.what+" left in the code")
				return &f
			}
		}
	}
	return nil
}
