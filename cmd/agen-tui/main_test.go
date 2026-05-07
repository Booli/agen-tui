package main

import "testing"

func TestParseForwardsRepeated(t *testing.T) {
	args := []string{"-L", "5137", "-L=8080:db:5432", "user@host:/tmp"}
	specs, rest, err := parseForwards(args)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("len(specs) = %d, want 2", len(specs))
	}
	if specs[0].LocalPort != 5137 || specs[1].LocalPort != 8080 || specs[1].RemoteHost != "db" {
		t.Errorf("specs = %+v", specs)
	}
	if len(rest) != 1 || rest[0] != "user@host:/tmp" {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseForwardsNoFlags(t *testing.T) {
	specs, rest, err := parseForwards([]string{"some/dir"})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Errorf("specs = %+v", specs)
	}
	if len(rest) != 1 || rest[0] != "some/dir" {
		t.Errorf("rest = %v", rest)
	}
}

func TestParseForwardsErrors(t *testing.T) {
	if _, _, err := parseForwards([]string{"-L"}); err == nil {
		t.Error("trailing -L should error")
	}
	if _, _, err := parseForwards([]string{"-L", "abc"}); err == nil {
		t.Error("invalid spec should error")
	}
}
