package tui

import (
	"context"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"
	"regexp"
	"strings"
	"testing"
)

func TestStructuredRolesAcrossRequestedLanguages(t *testing.T) {
	fixtures := []struct {
		name, source string
		roles        map[string]chroma.TokenType
	}{
		{"main.go", `package main
type Person struct { Name string }
func greet(person Person) string { label := person.Name; return label }
`, map[string]chroma.TokenType{"greet": chroma.NameFunction, "person": chroma.NameVariableMagic, "Person": chroma.NameClass, "label": chroma.NameVariable, "Name": chroma.NameProperty}},
		{"main.ts", `interface Person { name: string }
function greet(person: Person): string { const label = person.name; return label; }
`, map[string]chroma.TokenType{"greet": chroma.NameFunction, "person": chroma.NameVariableMagic, "Person": chroma.NameClass, "label": chroma.NameVariable, "name": chroma.NameProperty}},
		{"main.tsx", `type Props = { label: string }; const View = (props: Props) => <div title={props.label}>{props.label}</div>;`, map[string]chroma.TokenType{"View": chroma.NameFunction, "props": chroma.NameVariableMagic, "Props": chroma.NameClass, "div": chroma.NameTag, "title": chroma.NameAttribute}},
		{"main.js", `const greet = (person) => { const label = person.name; return label; };`, map[string]chroma.TokenType{"greet": chroma.NameFunction, "person": chroma.NameVariableMagic, "label": chroma.NameVariable, "name": chroma.NameProperty}},
		{"main.jsx", `function View(props) { return <div title={props.label}>{props.label}</div>; }`, map[string]chroma.TokenType{"View": chroma.NameFunction, "props": chroma.NameVariableMagic, "div": chroma.NameTag, "title": chroma.NameAttribute}},
		{"main.py", "class Person:\n    pass\n\ndef greet(person: Person):\n    label = person.name\n    return label\n", map[string]chroma.TokenType{"greet": chroma.NameFunction, "person": chroma.NameVariableMagic, "Person": chroma.NameClass, "label": chroma.NameVariable, "name": chroma.NameProperty}},
		{"main.html", `<section class="card" id="root"><span>Hello</span></section>`, map[string]chroma.TokenType{"section": chroma.NameTag, "class": chroma.NameAttribute, "span": chroma.NameTag, "id": chroma.NameAttribute}},
		{"main.css", `.card { color: red; --accent: blue; background: var(--accent); }`, map[string]chroma.TokenType{"card": chroma.NameClass, "color": chroma.NameProperty, "--accent": chroma.NameVariableGlobal}},
		{"main.json", `{"name": "vorq", "enabled": true, "count": 3}`, map[string]chroma.TokenType{`"name"`: chroma.NameProperty, `"enabled"`: chroma.NameProperty, `"count"`: chroma.NameProperty}},
	}
	color256(t)
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			spans := structuralSpans(context.Background(), fixture.name, fixture.source)
			found := map[string][]chroma.TokenType{}
			for _, span := range spans {
				text := fixture.source[span.start:span.end]
				found[text] = append(found[text], span.role)
			}
			for text, role := range fixture.roles {
				matched := false
				for _, actual := range found[text] {
					matched = matched || actual == role
				}
				if !matched {
					t.Errorf("%q want %s, got %v", text, role, found[text])
				}
			}
			rendered := highlightCodeSnippet(fixture.name, fixture.source, "Dark+")
			if ansi.Strip(rendered) != strings.TrimSuffix(fixture.source, "\n") {
				t.Fatal("highlight altered source")
			}
			if !strings.Contains(rendered, "\x1b[39;38;5;") {
				t.Fatal("no syntax colours")
			}
		})
	}
}

func TestRenderedSymbolCategoriesUseDifferentColours(t *testing.T) {
	color256(t)
	source := `package main
type Person struct{}
func greet(person Person) { label := person; println(label) }`
	pattern := regexp.MustCompile(`\x1b\[39;38;5;([0-9]+)m(greet|person|Person|label)`)
	for _, theme := range studioPalettes {
		rendered := highlightCodeSnippet("main.go", source, theme.name)
		colours := map[string]string{}
		for _, match := range pattern.FindAllStringSubmatch(rendered, -1) {
			colours[match[2]] = match[1]
		}
		unique := map[string]bool{}
		for _, name := range []string{"greet", "person", "Person", "label"} {
			if colours[name] == "" {
				t.Fatalf("%s missing colour for %s", theme.name, name)
			}
			unique[colours[name]] = true
		}
		if len(unique) != 4 {
			t.Fatalf("%s merges function, parameter, type or variable colours: %v", theme.name, colours)
		}
	}
}

func TestStructuredLocalScopesAndNoNamesFromStringsComments(t *testing.T) {
	source := `package main
func greet(name string) string {
    // name and greet are text
    { name := "literal name"; println(name) }
    return name
}`
	spans := structuralSpans(context.Background(), "main.go", source)
	var names []chroma.TokenType
	for _, span := range spans {
		if source[span.start:span.end] == "name" {
			names = append(names, span.role)
		}
	}
	want := []chroma.TokenType{chroma.NameVariableMagic, chroma.NameVariable, chroma.NameVariable, chroma.NameVariableMagic}
	if len(names) != len(want) {
		t.Fatalf("unexpected names: %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("scope classification %v want %v", names, want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if spans := structuralSpans(ctx, "main.go", source); len(spans) != 0 {
		t.Fatal("cancelled parse returned spans")
	}
}

func TestLocalConstructorAndCallableParameterKeepTheirRoles(t *testing.T) {
	source := "class Person:\n    pass\n\ndef apply(callback):\n    return callback(Person())\n"
	spans := structuralSpans(context.Background(), "main.py", source)
	for _, span := range spans {
		text := source[span.start:span.end]
		if text == "Person" && span.role != chroma.NameClass {
			t.Fatal("known class constructor lost type role")
		}
		if text == "callback" && span.role != chroma.NameVariableMagic {
			t.Fatal("called parameter lost parameter role")
		}
	}
}

func TestHTMLScriptStyleAndGoManifestAreHighlighted(t *testing.T) {
	color256(t)
	source := `<script>function greet(name) { return name; }</script><style>.card { color: red; }</style>`
	spans := structuralSpans(context.Background(), "main.html", source)
	found := map[string]chroma.TokenType{}
	for _, span := range spans {
		found[source[span.start:span.end]] = span.role
	}
	if found["greet"] != chroma.NameFunction || found["name"] != chroma.NameVariableMagic || found["color"] != chroma.NameProperty {
		t.Fatalf("embedded roles: %v", found)
	}
	manifest := "module example.com/vorq\n\ngo 1.27.1\nrequire example.com/lib v1.2.3 // dependency\n"
	got := highlightCodeSnippet("go.mod", manifest, "Dark+")
	if ansi.Strip(got) != strings.TrimSuffix(manifest, "\n") || !strings.Contains(got, "\x1b[") {
		t.Fatal("go.mod is plain or altered")
	}
}
