package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("AGEN_TUI_CONFIG", filepath.Join(t.TempDir(), "does-not-exist.json"))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if cfg != Default() {
		t.Errorf("missing file should yield defaults, got %+v", cfg)
	}
}

func TestLoadOverridesShowIcons(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"showIcons": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGEN_TUI_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.ShowIcons {
		t.Error("showIcons should be overridden to true")
	}
}

func TestLoadPartialKeepsOtherDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Only set collapseFolders; showIcons must keep its default.
	if err := os.WriteFile(path, []byte(`{"collapseFolders": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGEN_TUI_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.CollapseFolders {
		t.Error("collapseFolders should be overridden to true")
	}
	if cfg.ShowIcons {
		t.Error("showIcons should keep its default (false)")
	}
}

func TestLoadMalformedReturnsDefaultsAndError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{ not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGEN_TUI_CONFIG", path)

	cfg, err := Load()
	if err == nil {
		t.Fatal("malformed config should return an error")
	}
	if cfg != Default() {
		t.Errorf("malformed config should still yield defaults, got %+v", cfg)
	}
}

func TestPathHonorsXDG(t *testing.T) {
	t.Setenv("AGEN_TUI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/xdg/agen-tui/config.json"
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
