package ui

import (
	"bytes"
	"os"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var (
	hlFormatter chroma.Formatter = formatters.Get("terminal16")
	hlStyle     *chroma.Style    = pickStyle()
)

// pickStyle resolves the chroma style by AGEN_TUI_CHROMA_STYLE if set,
// falling back to a soft default that composes well with terminal
// palettes. Use `chroma --list` for the full set; popular options:
// monokai, dracula, nord, catppuccin-mocha, tokyo-night-dark, onedark.
func pickStyle() *chroma.Style {
	if name := os.Getenv("AGEN_TUI_CHROMA_STYLE"); name != "" {
		if s := styles.Get(name); s != nil {
			return s
		}
	}
	return styles.Get("monokai")
}

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
