// Package fuzzy is a tiny hand-rolled fuzzy matcher for filtering and
// ranking lists by a query string.
//
// Match attempts to find every rune of query inside target (case- and
// diacritic-insensitive, in order) and returns a score used to rank
// results, plus the list of rune-positions in target where each query
// rune matched (callers use those for inline highlighting).
//
// The greedy strategy is good enough for our scale (a few hundred to a
// few thousand items: file paths, tool names). Bonuses favour matches
// at the start of the string and after word boundaries, with a small
// length penalty so shorter targets rank higher when scores tie.
//
// The diacritic-insensitive matching (NFD → strip combining marks →
// NFC) is borrowed from lithammer/fuzzysearch (MIT). Lets typing
// "cafe" match "café", "naive" match "naïve", etc.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// foldChain decomposes runes into base + combining marks (NFD), drops
// the combining marks, then recomposes (NFC). Net effect: "é" → "e".
// strings.ToLower handles case folding afterward.
var foldChain = transform.Chain(
	norm.NFD,
	runes.Remove(runes.In(unicode.Mn)),
	norm.NFC,
)

// fold canonicalises s for matching: strips diacritics, lowercases.
// The returned string is what callers use for both query and target,
// so positions returned by Match are positions in the *folded* form
// of target (callers using highlights should fold the display string
// the same way, or accept minor index drift on accented input).
func fold(s string) string {
	out, _, err := transform.String(foldChain, s)
	if err != nil {
		out = s // graceful fallback: skip normalization
	}
	return strings.ToLower(out)
}

// Result is one matched item along with its score and the rune
// positions of each query character inside the item's key.
type Result[T any] struct {
	Item    T
	Score   int
	Indexes []int
}

// Match reports whether every rune of query appears in target in order
// (case-insensitive). When matched, score reflects ranking quality and
// indexes lists the rune positions in target that matched query.
//
// An empty query matches everything with score 0 and no indexes — that
// makes Filter act as identity when the user hasn't typed anything yet.
func Match(query, target string) (matched bool, score int, indexes []int) {
	if query == "" {
		return true, 0, nil
	}
	qRunes := []rune(fold(query))
	tRunes := []rune(fold(target))

	indexes = make([]int, 0, len(qRunes))
	qi := 0
	prevMatch := -2

	for ti := 0; ti < len(tRunes) && qi < len(qRunes); ti++ {
		if tRunes[ti] != qRunes[qi] {
			continue
		}
		cell := 1
		if ti == 0 {
			cell += 5
		} else if isBoundary(tRunes[ti-1]) {
			cell += 4
		}
		// Consecutive matches get the largest bonus — fuzzy tools
		// (fzf, etc.) heavily reward "all letters touching" since a
		// scattered subsequence almost always means a worse match.
		if prevMatch == ti-1 {
			cell += 5
		}
		score += cell
		indexes = append(indexes, ti)
		prevMatch = ti
		qi++
	}

	if qi < len(qRunes) {
		return false, 0, nil
	}
	score -= len(tRunes) / 10
	return true, score, indexes
}

// Filter ranks items by Match score against query. key extracts the
// search string from each item. Results are sorted by score descending,
// then by key length ascending (so concise matches win ties), then by
// stable-position to keep order deterministic.
func Filter[T any](query string, items []T, key func(T) string) []Result[T] {
	if query == "" {
		out := make([]Result[T], len(items))
		for i, it := range items {
			out[i] = Result[T]{Item: it}
		}
		return out
	}
	type indexed struct {
		Result[T]
		pos int
		key string
	}
	matched := make([]indexed, 0, len(items))
	for i, it := range items {
		k := key(it)
		ok, score, idx := Match(query, k)
		if !ok {
			continue
		}
		matched = append(matched, indexed{
			Result: Result[T]{Item: it, Score: score, Indexes: idx},
			pos:    i,
			key:    k,
		})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		a, b := matched[i], matched[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if len(a.key) != len(b.key) {
			return len(a.key) < len(b.key)
		}
		return a.pos < b.pos
	})
	out := make([]Result[T], len(matched))
	for i, m := range matched {
		out[i] = m.Result
	}
	return out
}

func isBoundary(r rune) bool {
	switch r {
	case '/', '_', '-', '.', ' ', ':', '\\':
		return true
	}
	return false
}
