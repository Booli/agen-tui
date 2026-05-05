package ui

import "testing"

func TestLCSDiff(t *testing.T) {
	cases := []struct {
		name     string
		a, b     []string
		wantKind string // concatenation of one char per row: ' ', '-', '+'
	}{
		{"identical", []string{"a", "b"}, []string{"a", "b"}, "  "},
		{"pure add", []string{}, []string{"x"}, "+"},
		{"pure delete", []string{"x"}, []string{}, "-"},
		{"middle change",
			[]string{"a", "b", "c"},
			[]string{"a", "B", "c"},
			" -+ "},
		{"insert line",
			[]string{"a", "c"},
			[]string{"a", "b", "c"},
			" + "},
		{"delete line",
			[]string{"a", "b", "c"},
			[]string{"a", "c"},
			" - "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := LCSDiff(tc.a, tc.b)
			var got string
			for _, r := range rows {
				got += string(r.Kind)
			}
			if got != tc.wantKind {
				t.Fatalf("kinds = %q, want %q\nrows = %v", got, tc.wantKind, rows)
			}
		})
	}
}
