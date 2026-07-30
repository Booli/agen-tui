// Package config loads user options from ~/.config/agen-tui/config.json.
//
// Absent files and absent fields both fall back to built-in defaults, so a
// partial config only overrides the keys it sets. The location can be
// overridden with $AGEN_TUI_CONFIG, and otherwise honors $XDG_CONFIG_HOME.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds the tunable options. JSON keys are camelCase.
type Config struct {
	// ShowIcons turns on the Nerd Font tree decorations: folder/file
	// device-icons, open/close chevrons, and small git-status glyphs.
	// Requires a Nerd Font in the terminal. Default off, so the tree keeps
	// its plain-text arrows and letter status codes unless opted in.
	ShowIcons bool `json:"showIcons"`

	// CollapseFolders starts the tree with every folder collapsed. Default
	// off, which keeps the current behavior: top-level folders and the
	// ancestors of any changed file are expanded on first load.
	CollapseFolders bool `json:"collapseFolders"`
}

// Default returns the built-in configuration used when no file is present.
func Default() Config {
	return Config{
		ShowIcons:       false,
		CollapseFolders: false,
	}
}

// Path returns the config file location: $AGEN_TUI_CONFIG if set, else
// $XDG_CONFIG_HOME/agen-tui/config.json, else ~/.config/agen-tui/config.json.
func Path() (string, error) {
	if p := os.Getenv("AGEN_TUI_CONFIG"); p != "" {
		return p, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "agen-tui", "config.json"), nil
}

// Load reads and parses the config file. A missing file yields the defaults
// with a nil error. A malformed file yields the defaults plus a non-nil
// error so the caller can warn without aborting startup.
func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	// Unmarshal over the defaults so omitted keys keep their default value.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}
