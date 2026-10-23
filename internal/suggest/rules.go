// Package suggest implements local, deterministic commit-message rules.
package suggest

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// ValidType reports whether a type is supported by the guided commit flow.
func ValidType(commitType string) bool {
	switch commitType {
	case "feat", "fix", "chore", "refactor", "docs", "test", "style", "perf":
		return true
	}
	return false
}

func isStaged(f domain.FileStatus) bool {
	return f.Staged != "" && f.Staged != " " && f.Staged != "?" && f.Staged != "U"
}

// InferScope selects a directory only when it represents a strict majority.
// Root files count towards the total, but never produce a scope.
func InferScope(files []domain.FileStatus) string {
	counts := map[string]int{}
	total := 0
	for _, f := range files {
		if !isStaged(f) {
			continue
		}
		total++
		parts := strings.Split(f.Path, "/")
		if len(parts) < 2 {
			continue
		}
		scope := parts[0]
		if (scope == "internal" || scope == "cmd") && len(parts) >= 3 {
			scope = parts[1]
		}
		// Directory names must not inject delimiters or controls into the header.
		if strings.IndexFunc(scope, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.'
		}) >= 0 {
			continue
		}
		counts[scope]++
	}
	for scope, count := range counts {
		if count > total/2 {
			return scope
		}
	}
	return ""
}

// GenerateSubject summarizes staged actions and names, independent of input order.
// Types label the Conventional Commit header; actions describe the actual changes.
func GenerateSubject(commitType string, files []domain.FileStatus) string {
	var staged []domain.FileStatus
	for _, f := range files {
		if isStaged(f) {
			staged = append(staged, f)
		}
	}
	sort.Slice(staged, func(i, j int) bool { return staged[i].Path < staged[j].Path })
	var groups []string
	for _, action := range []struct{ status, verb string }{
		{"A", "add"}, {"M", "update"}, {"D", "remove"}, {"R", "rename"}, {"C", "copy"}, {"T", "update type of"},
	} {
		var names []string
		for _, f := range staged {
			if f.Staged == action.status {
				name := path.Base(f.Path)
				if strings.IndexFunc(name, unicode.IsControl) >= 0 {
					name = strconv.QuoteToASCII(name)
				}
				if len([]rune(name)) > 40 {
					name = string([]rune(name)[:37]) + "..."
				}
				names = append(names, name)
			}
		}
		if len(names) > 2 {
			groups = append(groups, fmt.Sprintf("%s %d files", action.verb, len(names)))
		} else if len(names) > 0 {
			groups = append(groups, action.verb+" "+strings.Join(names, " and "))
		}
	}
	subject := strings.Join(groups, "; ")
	if subject == "" {
		return "update staged files"
	}
	if len([]rune(subject)) > 100 {
		return fmt.Sprintf("update %d staged files", len(staged))
	}
	return subject
}

// ProposeMessage builds a proposal using only the staged entries supplied.
func ProposeMessage(commitType string, files []domain.FileStatus) *domain.CommitProposal {
	header := commitType
	if scope := InferScope(files); scope != "" {
		header += "(" + scope + ")"
	}
	return &domain.CommitProposal{
		Subject:     header + ": " + GenerateSubject(commitType, files),
		Engine:      domain.EngineDeterministicRules,
		GeneratedAt: time.Now().UTC(),
	}
}
