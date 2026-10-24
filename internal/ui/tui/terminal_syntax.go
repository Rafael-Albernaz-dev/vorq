package tui

import (
	"bytes"
	"context"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
)

type syntaxTokenKey struct {
	style *chroma.Style
	token chroma.TokenType
}
type syntaxTokenStyle struct{ prefix, suffix string }

var syntaxTokenStyles sync.Map

// Let Chroma's terminal256 formatter select every style, once per token/theme.
// Streaming the source with WriteString avoids fmt/interface allocations for
// every token and retains the formatter's reset-at-newline behaviour.
func terminalTokenStyle(style *chroma.Style, token chroma.TokenType) (syntaxTokenStyle, error) {
	key := syntaxTokenKey{style, token}
	if cached, ok := syntaxTokenStyles.Load(key); ok {
		return cached.(syntaxTokenStyle), nil
	}
	var sample bytes.Buffer
	if err := formatters.Get("terminal256").Format(&sample, style, chroma.Literator(chroma.Token{Type: token, Value: "\x00"})); err != nil {
		return syntaxTokenStyle{}, err
	}
	prefix, suffix, _ := strings.Cut(sample.String(), "\x00")
	prefix = syntaxBackground.ReplaceAllString(prefix, "")
	prefix = strings.ReplaceAll(prefix, "\x1b[38;5;", "\x1b[39;38;5;")
	value := syntaxTokenStyle{prefix, suffix}
	actual, _ := syntaxTokenStyles.LoadOrStore(key, value)
	return actual.(syntaxTokenStyle), nil
}

func renderSyntaxTokens(ctx context.Context, style *chroma.Style, iterator chroma.Iterator, sourceLength int) (string, error) {
	var output strings.Builder
	output.Grow(min(sourceLength*4, 1024*1024))
	for token := iterator(); token != chroma.EOF; token = iterator() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		format, err := terminalTokenStyle(style, token.Type)
		if err != nil {
			return "", err
		}
		value := token.Value
		for {
			part, rest, newline := strings.Cut(value, "\n")
			if part != "" || newline {
				output.WriteString(format.prefix)
				output.WriteString(part)
				output.WriteString(format.suffix)
			}
			if !newline {
				break
			}
			output.WriteByte('\n')
			value = rest
		}
	}
	return output.String(), nil
}
