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
