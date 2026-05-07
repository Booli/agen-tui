package ui

import "testing"

func TestItoa(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-7, "-7"},
		{1000, "1000"},
	}
	for _, tc := range cases {
		if got := Itoa(tc.in); got != tc.want {
			t.Errorf("Itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPadLeft(t *testing.T) {
	if got := PadLeft("7", 4); got != "   7" {
		t.Errorf("got %q", got)
	}
	if got := PadLeft("longer", 3); got != "longer" {
		t.Errorf("PadLeft must not truncate, got %q", got)
	}
}

func TestTruncRunes(t *testing.T) {
	if got := TruncRunes("abcdef", 4); got != "abc…" {
		t.Errorf("got %q", got)
	}
	if got := TruncRunes("abc", 4); got != "abc" {
		t.Errorf("must not truncate when within width, got %q", got)
	}
	if got := TruncRunes("abc", 0); got != "" {
		t.Errorf("zero width should yield empty, got %q", got)
	}
}

func TestWrapLines(t *testing.T) {
	got := WrapLines("hello world", 5)
	want := []string{"hello", " worl", "d"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestWrapLinesPreservesANSI(t *testing.T) {
	// "\e[97mhello world\e[0m" wrapped at 5 — the escape sequence must not
	// be cut mid-stream. Visible content should be 5 cells per line, and
	// the resulting strings must NOT contain the literal "[97m" fragment
	// that the old rune-based wrapper would expose.
	in := "\x1b[97mhello world\x1b[0m"
	got := WrapLines(in, 5)
	if len(got) == 0 {
		t.Fatal("no output")
	}
	for i, line := range got {
		// The visible width measured ANSI-stripped should be ≤ 5.
		clean := stripANSI(line)
		if len([]rune(clean)) > 5 {
			t.Errorf("line %d (%q) has visible width %d, want ≤5", i, line, len(clean))
		}
		// No naked "[97m" or "[0m" — those would mean we cut an escape.
		if containsBare(line, "[97m") || containsBare(line, "[0m") {
			t.Errorf("line %d (%q) contains a broken escape fragment", i, line)
		}
	}
}

// stripANSI removes ESC [ ... m sequences. Crude but enough for this test.
func stripANSI(s string) string {
	var b []byte
	skip := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			skip = true
			continue
		}
		if skip {
			if c == 'm' {
				skip = false
			}
			continue
		}
		b = append(b, c)
	}
	return string(b)
}

// containsBare returns true if s contains needle without a preceding ESC.
// "ESC[97m" is fine; bare "[97m" leaking from a cut is not.
func containsBare(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			if i == 0 || s[i-1] != 0x1b {
				return true
			}
		}
	}
	return false
}
