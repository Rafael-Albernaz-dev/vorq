package tui

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	ts "github.com/tree-sitter/go-tree-sitter"
	css "github.com/tree-sitter/tree-sitter-css/bindings/go"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	html "github.com/tree-sitter/tree-sitter-html/bindings/go"
	js "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	jsonlang "github.com/tree-sitter/tree-sitter-json/bindings/go"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Structural roles enrich Chroma without executing a project or resolving
// packages. Bindings are local to the parsed file: this is not an LSP client.
type symbolScope struct {
	parent *symbolScope
	names  map[string]chroma.TokenType
}
type symbolSpan struct {
	start, end uint
	role       chroma.TokenType
	scope      *symbolScope
	resolve    bool
}

func symbolLanguage(filename string) (*ts.Language, string) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".go":
		return ts.NewLanguage(golang.Language()), "go"
	case ".ts", ".mts", ".cts":
		return ts.NewLanguage(typescript.LanguageTypescript()), "ts"
	case ".tsx":
		return ts.NewLanguage(typescript.LanguageTSX()), "ts"
	case ".js", ".jsx", ".mjs", ".cjs":
		return ts.NewLanguage(js.Language()), "js"
	case ".py", ".pyi":
		return ts.NewLanguage(python.Language()), "py"
	case ".html", ".htm":
		return ts.NewLanguage(html.Language()), "html"
	case ".css":
		return ts.NewLanguage(css.Language()), "css"
	case ".json", ".jsonc":
		return ts.NewLanguage(jsonlang.Language()), "json"
	default:
		return nil, ""
	}
}

func functionNode(kind string) bool {
	switch kind {
	case "function_declaration", "method_declaration", "function_definition", "function_expression", "generator_function_declaration", "generator_function", "arrow_function", "method_definition", "lambda":
		return true
	}
	return false
}

func structuralSpans(parent context.Context, filename, source string) []symbolSpan {
	language, dialect := symbolLanguage(filename)
	if language == nil || source == "" || parent.Err() != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 500*time.Millisecond)
	defer cancel()
	parser := ts.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(language); err != nil {
		return nil
	}
	bytes := []byte(source)
	// ParseWithOptions avoids the binding's deprecated ParseCtx goroutine and
	// polls cancellation within native parsing. Each command owns its parser.
	tree := parser.ParseWithOptions(func(offset int, _ ts.Point) []byte {
		if offset >= len(bytes) {
			return nil
		}
		return bytes[offset:]
	}, nil, &ts.ParseOptions{ProgressCallback: func(ts.ParseState) bool { return ctx.Err() != nil }})
	if tree == nil {
		return nil
	}
	defer tree.Close()
	if ctx.Err() != nil {
		return nil
	}
	rootScope := &symbolScope{names: map[string]chroma.TokenType{}}
	spans := []symbolSpan{}
	var visit func(*ts.Node, *symbolScope, bool, bool, string, int)
	visit = func(node *ts.Node, scope *symbolScope, parameter, typeContext bool, field string, depth int) {
		if node == nil || depth > 512 || ctx.Err() != nil {
			return
		}
		kind := node.Kind()
		if functionNode(kind) || (dialect != "py" && (kind == "block" || kind == "statement_block")) {
			scope = &symbolScope{parent: scope, names: map[string]chroma.TokenType{}}
		}
		if field == "parameters" || field == "parameter" || field == "receiver" || kind == "parameters" || kind == "formal_parameters" {
			parameter = true
		}
		if field == "type" || kind == "type_annotation" {
			typeContext = true
			parameter = false
		}
		if field == "value" || field == "right" || field == "default" {
			parameter = false
			typeContext = false
		}
		role, definition, resolve := nodeRole(node, field, parameter, typeContext, dialect, bytes)
		if role != 0 && node.EndByte() > node.StartByte() {
			text := node.Utf8Text(bytes)
			if definition {
				owner := scope
				// The name child is visited after the new function scope exists.
				if field == "name" && functionNode(kindOf(node.Parent())) && scope.parent != nil {
					owner = scope.parent
				}
				owner.names[text] = role
			}
			spans = append(spans, symbolSpan{node.StartByte(), node.EndByte(), role, scope, resolve})
		}
		if dialect == "html" && kind == "raw_text" {
			inline := ""
			switch kindOf(node.Parent()) {
			case "script_element":
				inline = "inline.js"
			case "style_element":
				inline = "inline.css"
			}
			if inline != "" {
				for _, span := range structuralSpans(ctx, inline, node.Utf8Text(bytes)) {
					span.start += node.StartByte()
					span.end += node.StartByte()
					spans = append(spans, span)
				}
			}
		}
		for i := uint(0); i < node.NamedChildCount(); i++ {
			visit(node.NamedChild(i), scope, parameter, typeContext, node.FieldNameForNamedChild(uint32(i)), depth+1)
		}
	}
	visit(tree.RootNode(), rootScope, false, false, "", 0)
	if ctx.Err() != nil {
		return nil
	}
	for i := range spans {
		span := &spans[i]
		if span.resolve {
			for scope := span.scope; scope != nil; scope = scope.parent {
				if role, ok := scope.names[source[span.start:span.end]]; ok {
					span.role = role
					break
				}
			}
		}
		span.scope = nil
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})
	return spans
}

func kindOf(node *ts.Node) string {
	if node == nil {
		return ""
	}
	return node.Kind()
}

func nodeRole(node *ts.Node, field string, parameter, typeContext bool, dialect string, source []byte) (chroma.TokenType, bool, bool) {
	kind, parent := node.Kind(), node.Parent()
	pk := kindOf(parent)
	switch kind {
	case "type_identifier", "predefined_type":
		return chroma.NameClass, field == "name", false
	case "package_identifier":
		return chroma.NameNamespace, false, false
	case "tag_name":
		return chroma.NameTag, false, false
	case "attribute_name":
		return chroma.NameAttribute, false, false
	case "class_name", "id_name":
		return chroma.NameClass, false, false
	case "property_name":
		if strings.HasPrefix(node.Utf8Text(source), "--") {
			return chroma.NameVariableGlobal, false, false
		}
		return chroma.NameProperty, false, false
	case "function_name":
		return chroma.NameFunction, false, false
	case "plain_value":
		if strings.HasPrefix(node.Utf8Text(source), "--") {
			return chroma.NameVariableGlobal, false, false
		}
	case "string":
		if dialect == "json" && pk == "pair" && field == "key" {
			return chroma.NameProperty, false, false
		}
	}
	switch kind {
	case "identifier", "field_identifier", "property_identifier", "shorthand_property_identifier", "shorthand_property_identifier_pattern", "private_property_identifier":
	default:
		return 0, false, false
	}
	if typeContext {
		return chroma.NameClass, false, false
	}
	if (functionNode(pk) || pk == "function_signature" || pk == "method_signature" || pk == "method_elem") && field == "name" {
		return chroma.NameFunction, true, false
	}
	if (pk == "class_definition" || pk == "class_declaration" || pk == "interface_declaration" || pk == "type_spec" || pk == "type_alias_declaration" || pk == "enum_declaration") && field == "name" {
		return chroma.NameClass, true, false
	}
	if pk == "jsx_opening_element" || pk == "jsx_closing_element" || pk == "jsx_self_closing_element" {
		if field == "name" {
			return chroma.NameTag, false, false
		}
	}
	if pk == "jsx_attribute" {
		return chroma.NameAttribute, false, false
	}
	if parameter && field != "type" {
		return chroma.NameVariableMagic, true, false
	}
	if (pk == "call_expression" || pk == "call") && field == "function" {
		return chroma.NameFunction, false, true
	}
	if kind == "field_identifier" || kind == "property_identifier" || kind == "private_property_identifier" || (pk == "attribute" && field == "attribute") {
		grandparent := parent.Parent()
		if (kindOf(grandparent) == "call_expression" || kindOf(grandparent) == "call") && grandparent.ChildByFieldName("function") != nil && grandparent.ChildByFieldName("function").Id() == parent.Id() {
			return chroma.NameFunction, false, false
		}
		return chroma.NameProperty, false, false
	}
	if pk == "variable_declarator" && field == "name" {
		if value := parent.ChildByFieldName("value"); value != nil && functionNode(value.Kind()) {
			return chroma.NameFunction, true, false
		}
		return chroma.NameVariable, true, false
	}
	if pk == "const_spec" && field == "name" {
		return chroma.NameConstant, true, false
	}
	if (pk == "var_spec" && field == "name") || (pk == "assignment" && field == "left") {
		return chroma.NameVariable, true, false
	}
	if pk == "expression_list" && kindOf(parent.Parent()) == "short_var_declaration" {
		if left := parent.Parent().ChildByFieldName("left"); left != nil && left.Id() == parent.Id() {
			return chroma.NameVariable, true, false
		}
	}
	return chroma.NameVariable, false, true
}

// Split Chroma tokens at AST byte boundaries; all text stays byte-for-byte
// identical and all offsets refer to the already sanitised UTF-8 source.
func withSymbolRoles(lexical chroma.Iterator, spans []symbolSpan) chroma.Iterator {
	var current chroma.Token
	var offset uint
	index := 0
	return func() chroma.Token {
		for current.Value == "" {
			current = lexical()
			if current == chroma.EOF {
				return chroma.EOF
			}
		}
		for index < len(spans) && spans[index].end <= offset {
			index++
		}
		end := offset + uint(len(current.Value))
		role := current.Type
		if index < len(spans) && spans[index].start < end {
			span := spans[index]
			if span.start > offset {
				end = span.start
			} else {
				role = span.role
				end = min(end, span.end)
			}
		}
		length := end - offset
		token := chroma.Token{Type: role, Value: current.Value[:length]}
		current.Value = current.Value[length:]
		offset = end
		return token
	}
}
