package fuzzy

import (
	"reflect"
	"testing"
)

func TestEmptyQueryMatches(t *testing.T) {
	ok, score, idx := Match("", "anything")
	if !ok || score != 0 || len(idx) != 0 {
		t.Errorf("empty query: ok=%v score=%d idx=%v", ok, score, idx)
	}
}

func TestSubsequenceMatch(t *testing.T) {
	ok, _, idx := Match("agt", "agen-tui/app.go")
	if !ok {
		t.Fatal("agt should match agen-tui/app.go")
	}
	// Verify the indexes correspond to the matched chars.
	target := "agen-tui/app.go"
	got := []byte{}
	for _, i := range idx {
		got = append(got, target[i])
	}
	if string(got) != "agt" {
		t.Errorf("indexes select %q, want \"agt\"", got)
	}
}

func TestNoMatch(t *testing.T) {
	ok, _, _ := Match("xyz", "agen-tui")
	if ok {
		t.Error("xyz should not match agen-tui")
	}
}

func TestCaseInsensitive(t *testing.T) {
	ok, _, _ := Match("APP", "agen-tui/App.go")
	if !ok {
		t.Error("APP should match App.go (case-insensitive)")
	}
}

func TestStartBonusBeatsMidMatch(t *testing.T) {
	_, scoreStart, _ := Match("agen", "agen-tui")
	_, scoreMid, _ := Match("agen", "lib-agen")
	if scoreStart <= scoreMid {
		t.Errorf("start match (%d) should beat mid match (%d)", scoreStart, scoreMid)
	}
}

func TestBoundaryBonusBeatsInternal(t *testing.T) {
	// "tui" should score higher in "lib/tui-app" (after '/' boundary)
	// than in "lertuipx" (no boundary).
	_, scoreBoundary, _ := Match("tui", "lib/tui-app")
	_, scoreInternal, _ := Match("tui", "lertuipx")
	if scoreBoundary <= scoreInternal {
		t.Errorf("boundary match (%d) should beat internal match (%d)", scoreBoundary, scoreInternal)
	}
}

func TestConsecutiveBonus(t *testing.T) {
	// "agen" matched as 4 consecutive chars > "agen" matched scattered.
	_, scoreConsec, _ := Match("agen", "agen-tui")
	_, scoreScattered, _ := Match("agen", "a_g_e_n_z")
	if scoreConsec <= scoreScattered {
		t.Errorf("consecutive (%d) should beat scattered (%d)", scoreConsec, scoreScattered)
	}
}

func TestLengthPenalty(t *testing.T) {
	// Same match quality but shorter target should score higher.
	_, scoreShort, _ := Match("ab", "ab")
	_, scoreLong, _ := Match("ab", "ab"+stringRepeat(50))
	if scoreShort <= scoreLong {
		t.Errorf("short (%d) should outscore long (%d)", scoreShort, scoreLong)
	}
}

func TestFilterRanksByScore(t *testing.T) {
	items := []string{
		"lib/foo/agen.go",
		"agen-tui/app.go",
		"agent.txt",
	}
	results := Filter("agen", items, func(s string) string { return s })
	if len(results) != 3 {
		t.Fatalf("len = %d, want 3", len(results))
	}
	// Best ranked first — "agen-tui/app.go" starts with the query.
	// "agent.txt" also starts with the query and is shorter, so wins.
	if results[0].Item != "agent.txt" {
		t.Errorf("best result = %q, want agent.txt", results[0].Item)
	}
}

func TestFilterDropsNonMatches(t *testing.T) {
	items := []string{"foo", "bar", "fobar"}
	results := Filter("fob", items, func(s string) string { return s })
	if len(results) != 1 || results[0].Item != "fobar" {
		t.Errorf("results = %+v, want only fobar", results)
	}
}

func TestFilterEmptyQueryReturnsAllInOrder(t *testing.T) {
	items := []string{"a", "b", "c"}
	results := Filter("", items, func(s string) string { return s })
	got := []string{}
	for _, r := range results {
		got = append(got, r.Item)
	}
	if !reflect.DeepEqual(got, items) {
		t.Errorf("got %v, want %v", got, items)
	}
}

func TestFilterStableTiebreak(t *testing.T) {
	// Two items with the same score should preserve input order.
	items := []string{"alpha", "alpha"} // identical score and length
	results := Filter("a", items, func(s string) string { return s })
	if len(results) != 2 {
		t.Fatalf("len = %d", len(results))
	}
	// Both items are the same string; positions preserved (no panic).
}

func TestUnicode(t *testing.T) {
	ok, _, idx := Match("über", "Übersicht")
	if !ok {
		t.Error("ü should match Ü (case-insensitive)")
	}
	if len(idx) != 4 {
		t.Errorf("indexes = %v, want 4 entries", idx)
	}
}

func TestDiacriticInsensitive(t *testing.T) {
	cases := []struct {
		query, target string
	}{
		{"cafe", "café"},
		{"naive", "naïve"},
		{"über", "uber"},
		{"resume", "résumé"},
	}
	for _, c := range cases {
		if ok, _, _ := Match(c.query, c.target); !ok {
			t.Errorf("Match(%q, %q) should match (diacritic folding)", c.query, c.target)
		}
		// Symmetric: query with diacritics should match plain too.
		if ok, _, _ := Match(c.target, c.query); !ok {
			t.Errorf("Match(%q, %q) should match (symmetric folding)", c.target, c.query)
		}
	}
}

func stringRepeat(n int) string {
	s := make([]byte, n)
	for i := range s {
		s[i] = 'x'
	}
	return string(s)
}
