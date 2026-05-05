package ui

import (
	"bytes"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	hlFormatter chroma.Formatter = formatters.Get("terminal256")
	hlStyle     *chroma.Style    = styles.Get("github-dark")
)

// Highlight returns ANSI-styled source code, picking a chroma lexer by
// the given filename's extension. If no lexer matches or the source is
// empty, returns text unchanged. Safe to call with a nil/empty filename
// — the analyser fallback will pick a lexer when it can.
func Highlight(text, filename string) string {
	if text == "" {
		return text
	}
	var lexer chroma.Lexer
	if filename != "" {
		lexer = lexers.Match(filename)
	}
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		return text
	}
	iter, err := lexer.Tokenise(nil, text)
	if err != nil {
		return text
	}
	var buf bytes.Buffer
	if err := hlFormatter.Format(&buf, hlStyle, iter); err != nil {
		return text
	}
	return buf.String()
}
