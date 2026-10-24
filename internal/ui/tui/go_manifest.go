package tui

import "github.com/alecthomas/chroma/v2"

// go.mod/go.work are manifests, not Go source. Chroma's Go grammar cannot
// classify module directives; provide that explicit filename fallback.
var goManifestLexer = chroma.MustNewLexer(&chroma.Config{Name: "Go module manifest"}, func() chroma.Rules {
	return chroma.Rules{"root": {
		{Pattern: `\s+`, Type: chroma.TextWhitespace},
		{Pattern: `//[^\n]*`, Type: chroma.CommentSingle},
		{Pattern: `\b(module|go|toolchain|require|replace|exclude|retract|use|tool|godebug)\b`, Type: chroma.KeywordNamespace},
		{Pattern: `h1:[A-Za-z0-9+/=]+`, Type: chroma.LiteralString},
		{Pattern: `\bv?\d+(?:\.[0-9A-Za-z_-]+)+(?:\+[A-Za-z0-9.-]+)?`, Type: chroma.LiteralNumber},
		{Pattern: `"(?:[^"\\]|\\.)*"`, Type: chroma.LiteralString},
		{Pattern: `=>|[()]`, Type: chroma.Punctuation},
		{Pattern: `[^\s()]+`, Type: chroma.NameNamespace},
	}}
})
