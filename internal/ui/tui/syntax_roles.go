package tui

import (
	"github.com/alecthomas/chroma/v2"
	ts "github.com/tree-sitter/go-tree-sitter"
	"strings"
)

// Grammar leaves supply lexical categories, including recoverable incomplete
// source. Symbol classification takes precedence; unrecognised ranges remain
// plain text. Unsupported or unavailable grammars use the Chroma fallback.
func lexicalNodeRole(node *ts.Node, source []byte) chroma.TokenType {
	kind := node.Kind()
	switch kind {
	case "comment", "line_comment", "block_comment":
		text := node.Utf8Text(source)
		if strings.HasPrefix(text, "//") || strings.HasPrefix(text, "#") {
			return chroma.CommentSingle
		}
		return chroma.CommentMultiline
	case "string", "template_string":
		for i := uint(0); i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i).Kind()
			if child == "interpolation" || child == "template_substitution" {
				return 0
			}
		}
		return chroma.LiteralString
	case "interpreted_string_literal", "raw_string_literal", "string_literal", "quoted_attribute_value", "attribute_value", "string_content", "string_fragment", "string_start", "string_end":
		return chroma.LiteralString
	case "escape_sequence":
		return chroma.LiteralStringEscape
	case "rune_literal":
		return chroma.LiteralStringChar
	case "int_literal", "integer", "integer_value":
		return chroma.LiteralNumberInteger
	case "float_literal", "float", "float_value":
		return chroma.LiteralNumberFloat
	case "imaginary_literal", "number":
		return chroma.LiteralNumber
	case "color_value":
		return chroma.LiteralNumberHex
	case "true", "false", "nil", "null", "none":
		return chroma.KeywordConstant
	case "entity":
		return chroma.NameEntity
	}
	if node.IsNamed() {
		return 0
	}
	switch kind {
	case "package", "import", "from", "namespace", "using":
		return chroma.KeywordNamespace
	case "var", "let", "const", "func", "function", "def", "class", "struct", "type", "interface", "enum", "map", "chan":
		return chroma.KeywordDeclaration
	case "if", "else", "elif", "for", "while", "return", "switch", "case", "default", "break", "continue", "defer", "go", "select", "range", "fallthrough", "goto", "try", "except", "finally", "raise", "throw", "catch", "new", "delete", "typeof", "instanceof", "in", "of", "is", "not", "and", "or", "as", "with", "yield", "lambda", "async", "await", "pass", "assert", "del", "global", "nonlocal", "extends", "implements", "export", "public", "private", "protected", "static", "readonly", "abstract", "declare", "keyof", "infer", "satisfies", "get", "set":
		return chroma.Keyword
	case "(", ")", "[", "]", "{", "}", ".", ",", ";", ":":
		return chroma.Punctuation
	case "`", "\"", "'":
		return chroma.LiteralStringDelimiter
	}
	if strings.ContainsAny(kind, "+-*/%=!<>|&^~?") {
		return chroma.Operator
	}
	return 0
}

func plainSourceIterator(source string) chroma.Iterator {
	remaining := source
	return func() chroma.Token {
		if remaining == "" {
			return chroma.EOF
		}
		value := remaining
		remaining = ""
		return chroma.Token{Type: chroma.Text, Value: value}
	}
}
