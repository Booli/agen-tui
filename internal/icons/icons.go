// Package icons maps file names and extensions to Nerd Font glyphs and
// colors, in the style of nvim-web-devicons / snacks.nvim's explorer.
//
// Requires a Nerd Font (https://www.nerdfonts.com) in the terminal for the
// glyphs to render. Colors are device-icon brand colors (truecolor hex);
// terminals without truecolor degrade gracefully to the nearest color.
package icons

import (
	"path/filepath"
	"strings"
)

// Icon is a glyph plus an optional foreground color (hex, "" = inherit).
type Icon struct {
	Glyph string
	Color string
}

const folderColor = "#7aa2f7" // muted blue, matches snacks' directory hue

var (
	// FolderClosed / FolderOpen double as the expand/collapse indicator:
	// the glyph changes with the folder's state instead of a separate arrow.
	FolderClosed = Icon{"", folderColor} // nf-fa-folder
	FolderOpen   = Icon{"", folderColor} // nf-fa-folder_open

	defaultFile = Icon{"", ""} // nf-fa-file
)

// byName matches whole (lowercased) file names before falling back to ext.
var byName = map[string]Icon{
	"go.mod":            {"", "#00add8"},
	"go.sum":            {"", "#00add8"},
	".gitignore":        {"", "#f14c28"},
	".gitattributes":    {"", "#f14c28"},
	".gitmodules":       {"", "#f14c28"},
	"dockerfile":        {"", "#458ee6"},
	".dockerignore":     {"", "#458ee6"},
	"makefile":          {"", "#6d8086"},
	"license":           {"", "#cb4b16"},
	"package.json":      {"", "#8cc84b"},
	"package-lock.json": {"", "#8cc84b"},
	".env":              {"", "#faf743"},
	".editorconfig":     {"", "#6d8086"},
}

// byExt matches the lowercased extension (including leading dot).
var byExt = map[string]Icon{
	".go":       {"", "#00add8"},
	".js":       {"", "#f7df1e"},
	".mjs":      {"", "#f7df1e"},
	".cjs":      {"", "#f7df1e"},
	".jsx":      {"", "#519aba"},
	".ts":       {"", "#3178c6"},
	".tsx":      {"", "#3178c6"},
	".json":     {"", "#cbcb41"},
	".md":       {"", "#519aba"},
	".markdown": {"", "#519aba"},
	".py":       {"", "#ffbc03"},
	".rs":       {"", "#dea584"},
	".rb":       {"", "#701516"},
	".html":     {"", "#e34c26"},
	".htm":      {"", "#e34c26"},
	".css":      {"", "#563d7c"},
	".scss":     {"", "#cc6699"},
	".sass":     {"", "#cc6699"},
	".vue":      {"", "#42b883"},
	".c":        {"", "#599eff"},
	".h":        {"", "#a074c4"},
	".cpp":      {"", "#519aba"},
	".cc":       {"", "#519aba"},
	".hpp":      {"", "#a074c4"},
	".cs":       {"", "#596706"},
	".java":     {"", "#cc3e44"},
	".kt":       {"", "#7f52ff"},
	".php":      {"", "#a074c4"},
	".lua":      {"", "#51a0cf"},
	".swift":    {"", "#e37933"},
	".sh":       {"", "#4d5a5e"},
	".bash":     {"", "#4d5a5e"},
	".zsh":      {"", "#4d5a5e"},
	".fish":     {"", "#4d5a5e"},
	".yml":      {"", "#6d8086"},
	".yaml":     {"", "#6d8086"},
	".toml":     {"", "#6d8086"},
	".ini":      {"", "#6d8086"},
	".conf":     {"", "#6d8086"},
	".sql":      {"", "#dad8d8"},
	".db":       {"", "#dad8d8"},
	".png":      {"", "#a074c4"},
	".jpg":      {"", "#a074c4"},
	".jpeg":     {"", "#a074c4"},
	".gif":      {"", "#a074c4"},
	".webp":     {"", "#a074c4"},
	".ico":      {"", "#a074c4"},
	".svg":      {"", "#ffb13b"},
	".pdf":      {"", "#b30b00"},
	".zip":      {"", "#eca517"},
	".tar":      {"", "#eca517"},
	".gz":       {"", "#eca517"},
	".tgz":      {"", "#eca517"},
	".txt":      {"", ""},
	".log":      {"", "#6d8086"},
}

// FileIcon returns the icon for a file given its base name. It matches the
// whole name first (e.g. "go.mod", "Dockerfile"), then the extension, then
// falls back to a generic file glyph.
func FileIcon(name string) Icon {
	lower := strings.ToLower(name)
	if ic, ok := byName[lower]; ok {
		return ic
	}
	if ic, ok := byExt[strings.ToLower(filepath.Ext(name))]; ok {
		return ic
	}
	return defaultFile
}
