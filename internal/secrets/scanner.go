package secrets

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// AllowlistFile is the versioned exceptions file at the workspace root.
const AllowlistFile = ".vorqsecrets-allow"

type allowEntry struct {
	glob  string
	rules map[string]bool // empty: every rule
}

// Scanner applies the built-in rules plus the repository's exceptions.
type Scanner struct{ allow []allowEntry }

// NewScanner parses .vorqsecrets-allow content. Each non-comment line is a
// path glob, optionally followed by rule IDs: "testdata/** jwt,generic-secret".
// "*" alone as the glob applies the rules everywhere. Globs without "/" match
// the base name at any depth; "**" matches any number of directories.
func NewScanner(allowlist string) (*Scanner, error) {
	s := &Scanner{}
	for i, raw := range strings.Split(allowlist, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.ReplaceAll(line, ",", " "))
		if _, err := path.Match(strings.ReplaceAll(fields[0], "**", "*"), ""); err != nil {
			return nil, fmt.Errorf("%s:%d: invalid glob %q", AllowlistFile, i+1, fields[0])
		}
		entry := allowEntry{glob: strings.TrimPrefix(fields[0], "./"), rules: map[string]bool{}}
		for _, rule := range fields[1:] {
			entry.rules[rule] = true
		}
		s.allow = append(s.allow, entry)
	}
	return s, nil
}

func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, _ := path.Match(pattern[0], name[0])
	return ok && matchSegments(pattern[1:], name[1:])
}

func globMatch(glob, p string) bool {
	if glob == "*" {
		return true
	}
	if !strings.Contains(glob, "/") {
		ok, _ := path.Match(glob, path.Base(p))
		return ok
	}
	return matchSegments(strings.Split(strings.TrimSuffix(glob, "/"), "/"), strings.Split(p, "/")) ||
		(strings.HasSuffix(glob, "/") && strings.HasPrefix(p, glob))
}

func (s *Scanner) allowed(f domain.GuardFinding) bool {
	for _, e := range s.allow {
		if (len(e.rules) == 0 || e.rules[f.Rule]) && globMatch(e.glob, f.Path) {
			return true
		}
	}
	return false
}

func (s *Scanner) keep(in []domain.GuardFinding) []domain.GuardFinding {
	var out []domain.GuardFinding
	for _, f := range in {
		if !s.allowed(f) {
			out = append(out, f)
		}
	}
	return out
}

// fileScan checks added lines of one file. Line numbers refer to the new version.
type fileScan struct {
	path     string
	findings []domain.GuardFinding
	conflict *domain.GuardFinding
}

func (fs *fileScan) line(text string, n int) {
	if f := scanSecretLine(fs.path, text, n); f != nil {
		fs.findings = append(fs.findings, *f)
	}
	if f := debugFinding(fs.path, text, n); f != nil {
		fs.findings = append(fs.findings, *f)
	}
	// "=======" alone is a valid Markdown/RST underline; the start and end
	// markers are unambiguous, so only they are reported.
	if fs.conflict == nil && (conflictStart.MatchString(text) || conflictEnd.MatchString(text)) {
		f := finding("conflict-marker", domain.GuardConflict, true, fs.path, n, strings.TrimSpace(text), "merge conflict marker")
		fs.conflict = &f
	}
}

func (fs *fileScan) done() []domain.GuardFinding {
	if fs.conflict != nil {
		fs.findings = append(fs.findings, *fs.conflict)
	}
	return fs.findings
}

// PathFinding reports a secret-bearing file name, or nil.
func (s *Scanner) PathFinding(p string) *domain.GuardFinding {
	why := sensitivePath(p)
	if why == "" {
		return nil
	}
	f := finding("sensitive-file", domain.GuardSecret, true, p, 0, "", why)
	if s.allowed(f) {
		return nil
	}
	return &f
}

// ScanContent checks a whole file (new file or AI payload content).
func (s *Scanner) ScanContent(p string, content string) []domain.GuardFinding {
	var out []domain.GuardFinding
	if f := s.PathFinding(p); f != nil {
		out = append(out, *f)
	}
	fs := &fileScan{path: p}
	for i, line := range strings.Split(content, "\n") {
		fs.line(strings.TrimSuffix(line, "\r"), i+1)
	}
	return append(out, s.keep(fs.done())...)
}

// HasSecret reports whether findings contain a blocking secret.
func HasSecret(findings []domain.GuardFinding) bool {
	for _, f := range findings {
		if f.Category == domain.GuardSecret && f.Blocking {
			return true
		}
	}
	return false
}

func unquotePath(raw string) string {
	if strings.HasPrefix(raw, `"`) {
		if s, err := strconv.Unquote(raw); err == nil {
			return s
		}
	}
	return raw
}

// headerPath extracts the new path from "diff --git a/X b/Y".
func headerPath(header string) string {
	rest := strings.TrimPrefix(header, "diff --git ")
	if strings.HasPrefix(rest, `"`) {
		// a/ quoted: find the closing quote, then the b/ part.
		for i := 1; i < len(rest); i++ {
			if rest[i] == '\\' {
				i++
				continue
			}
			if rest[i] == '"' {
				return strings.TrimPrefix(unquotePath(strings.TrimSpace(rest[i+1:])), "b/")
			}
		}
		return ""
	}
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return unquotePath(rest[i+1:])[2:]
	}
	if i := strings.Index(rest, ` "b/`); i >= 0 {
		return strings.TrimPrefix(unquotePath(rest[i+1:]), "b/")
	}
	return ""
}

// PatchPath is the file a single-file patch section describes: the new path,
// or the old one for a deletion.
func PatchPath(section string) string {
	p := ""
	for _, line := range strings.Split(section, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if p != "" {
				return p
			}
			p = headerPath(line)
		case strings.HasPrefix(line, "--- a/") || strings.HasPrefix(line, `--- "a/`):
			if old := strings.TrimPrefix(unquotePath(strings.TrimPrefix(line, "--- ")), "a/"); old != "" {
				p = old
			}
		case strings.HasPrefix(line, "+++ ") && line != "+++ /dev/null":
			return strings.TrimPrefix(unquotePath(strings.TrimPrefix(line, "+++ ")), "b/")
		case strings.HasPrefix(line, "@@ "):
			return p
		}
	}
	return p
}

// ScanPatch checks a unified Git patch: path rules for every file that
// exists after the change and content rules for added lines only.
func (s *Scanner) ScanPatch(patch string) []domain.GuardFinding {
	var out []domain.GuardFinding
	var cur *fileScan
	deleted := false
	newLine := 0
	inHunk := false
	flush := func() {
		if cur == nil {
			return
		}
		if !deleted && cur.path != "" {
			if f := s.PathFinding(cur.path); f != nil {
				out = append(out, *f)
			}
		}
		out = append(out, s.keep(cur.done())...)
		cur = nil
	}
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			cur, deleted, inHunk = &fileScan{path: headerPath(line)}, false, false
			continue
		case cur == nil:
			continue
		case !inHunk && strings.HasPrefix(line, "deleted file mode"):
			deleted = true
			continue
		case !inHunk && strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p != "/dev/null" {
				cur.path = strings.TrimPrefix(unquotePath(p), "b/")
			}
			continue
		case !inHunk && strings.HasPrefix(line, "rename to "):
			cur.path = unquotePath(strings.TrimPrefix(line, "rename to "))
			continue
		case strings.HasPrefix(line, "@@ "):
			inHunk = true
			newLine = hunkStart(line)
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			cur.line(strings.TrimSuffix(line[1:], "\r"), newLine)
			newLine++
		case strings.HasPrefix(line, " "):
			newLine++
		}
	}
	flush()
	return out
}

func hunkStart(header string) int {
	// @@ -a,b +c,d @@
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 1
	}
	start := strings.SplitN(strings.TrimPrefix(fields[2], "+"), ",", 2)[0]
	n, err := strconv.Atoi(start)
	if err != nil {
		return 1
	}
	return n
}

// LargeFile reports an added/modified file above LargeFileLimit.
func (s *Scanner) LargeFile(p string, size int64) *domain.GuardFinding {
	if size <= LargeFileLimit {
		return nil
	}
	f := finding("large-file", domain.GuardLargeFile, false, p, 0, "", fmt.Sprintf("file has %.1f MiB (limit %d MiB)", float64(size)/(1024*1024), LargeFileLimit/(1024*1024)))
	if s.allowed(f) {
		return nil
	}
	return &f
}

// CaseConflicts reports paths that differ only by letter case, when at
// least one of them is in changed (pre-existing collisions are not repeated).
func (s *Scanner) CaseConflicts(all []string, changed []string) []domain.GuardFinding {
	groups := map[string][]string{}
	for _, p := range all {
		k := strings.ToLower(p)
		groups[k] = append(groups[k], p)
	}
	seen := map[string]bool{}
	var out []domain.GuardFinding
	for _, p := range changed {
		k := strings.ToLower(p)
		if seen[k] {
			continue
		}
		seen[k] = true
		names := uniq(groups[k])
		if len(names) < 2 {
			continue
		}
		f := finding("case-conflict", domain.GuardCaseConflict, false, p, 0, "", "differs only by letter case from "+strings.Join(others(names, p), ", "))
		if !s.allowed(f) {
			out = append(out, f)
		}
	}
	return out
}

func uniq(in []string) []string {
	m := map[string]bool{}
	var out []string
	for _, v := range in {
		if !m[v] {
			m[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func others(names []string, p string) []string {
	var out []string
	for _, n := range names {
		if n != p {
			out = append(out, n)
		}
	}
	return out
}
