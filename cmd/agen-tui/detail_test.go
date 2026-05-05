package main

import (
	"testing"

	"github.com/pimrutgers/agen-tui/internal/filetree"
)

func TestExpandTouchedAncestors(t *testing.T) {
	// build:
	//   a/
	//     b/
	//       c.go (modified)
	//     clean.go
	//   d/
	//     e.go (untracked)
	//   top.go (modified)
	root := &filetree.Node{IsDir: true, Children: []*filetree.Node{
		{Name: "a", Path: "a", IsDir: true, Children: []*filetree.Node{
			{Name: "b", Path: "a/b", IsDir: true, Children: []*filetree.Node{
				{Name: "c.go", Path: "a/b/c.go", XY: " M"},
			}},
			{Name: "clean.go", Path: "a/clean.go"},
		}},
		{Name: "d", Path: "d", IsDir: true, Children: []*filetree.Node{
			{Name: "e.go", Path: "d/e.go", XY: "??"},
		}},
		{Name: "top.go", Path: "top.go", XY: " M"},
	}}

	expanded := map[string]bool{}
	expandTouchedAncestors(root, expanded)

	wantTrue := []string{"a", "a/b", "d"}
	for _, k := range wantTrue {
		if !expanded[k] {
			t.Errorf("expected %q to be expanded", k)
		}
	}
	// top.go has no parent dir to expand — must not produce a "" entry
	if expanded[""] {
		t.Error("synthetic root should never be marked expanded")
	}
	// clean dir-with-only-clean-children should NOT be expanded — but in
	// this fixture "a" *is* expanded (because of a/b/c.go).
	// Add a separate clean subtree to exercise the negative case:
	clean := &filetree.Node{IsDir: true, Children: []*filetree.Node{
		{Name: "x", Path: "x", IsDir: true, Children: []*filetree.Node{
			{Name: "y.go", Path: "x/y.go"},
		}},
	}}
	expanded2 := map[string]bool{}
	expandTouchedAncestors(clean, expanded2)
	if len(expanded2) != 0 {
		t.Errorf("clean tree should produce no expansions, got %v", expanded2)
	}
}

