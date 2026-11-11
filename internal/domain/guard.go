package domain

import "fmt"

// Guardrail categories (VORQ-018).
const (
	GuardSecret       = "secret"
	GuardConflict     = "conflict"
	GuardLargeFile    = "large-file"
	GuardDebug        = "debug"
	GuardCaseConflict = "case-conflict"
	GuardUnverified   = "unverified"
)

// GuardFinding is one local safety finding. Excerpt is always masked: the
// detected value itself is never stored, shown or sent anywhere.
type GuardFinding struct {
	Rule     string `json:"rule"`
	Category string `json:"category"`
	Blocking bool   `json:"blocking"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Commit   string `json:"commit,omitempty"`
	Excerpt  string `json:"excerpt,omitempty"`
	Message  string `json:"message"`
}

// Location renders commit, path and line compactly.
func (f GuardFinding) Location() string {
	loc := f.Path
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", loc, f.Line)
	}
	if f.Commit != "" {
		short := f.Commit
		if len(short) > 7 {
			short = short[:7]
		}
		loc = short + " " + loc
	}
	return loc
}

// GuardReport is the result of a guardrail check.
type GuardReport struct {
	Findings []GuardFinding `json:"findings"`
	Commits  int            `json:"commits,omitempty"` // commits evaluated (push)
}

func (r *GuardReport) filter(blocking bool) []GuardFinding {
	if r == nil {
		return nil
	}
	var out []GuardFinding
	for _, f := range r.Findings {
		if f.Blocking == blocking {
			out = append(out, f)
		}
	}
	return out
}

func (r *GuardReport) Blocking() []GuardFinding { return r.filter(true) }
func (r *GuardReport) Warnings() []GuardFinding { return r.filter(false) }
func (r *GuardReport) HasBlocking() bool        { return len(r.Blocking()) > 0 }

// GuardBlockedError stops a commit or push with blocking findings. The
// interface may retry with an explicit, typed override for that operation.
type GuardBlockedError struct {
	Operation string
	Report    GuardReport
}

func (e *GuardBlockedError) Error() string {
	return fmt.Sprintf("%s blocked by %d safety finding(s); fix them or confirm an explicit override", e.Operation, len(e.Report.Blocking()))
}

// GuardOverrideWord is what the user types to override blocking findings.
const GuardOverrideWord = "CONFIRMAR"
