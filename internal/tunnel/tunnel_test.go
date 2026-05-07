package tunnel

import (
	"testing"
)

func TestParseSpecShorthand(t *testing.T) {
	s, err := ParseSpec("5137")
	if err != nil {
		t.Fatal(err)
	}
	if s.LocalPort != 5137 || s.RemotePort != 5137 || s.RemoteHost != "localhost" {
		t.Errorf("got %+v", s)
	}
}

func TestParseSpecFull(t *testing.T) {
	s, err := ParseSpec("8080:db.internal:5432")
	if err != nil {
		t.Fatal(err)
	}
	if s.LocalPort != 8080 || s.RemoteHost != "db.internal" || s.RemotePort != 5432 {
		t.Errorf("got %+v", s)
	}
}

func TestParseSpecInvalid(t *testing.T) {
	cases := []string{
		"",
		"abc",
		"5137:host",
		"0",
		"99999",
		":host:80",
		"80:host:abc",
		"80::8080",                   // empty remote host
		"80: :8080",                  // whitespace-only remote host
		"-1:host:80",                 // negative port
		"80:host with space:8080",    // space in remote host
		"80:host;rm:8080",            // shell-meta in remote host
		"80:`hostname`:8080",         // backticks
		"80:$HOST:8080",              // env-expansion attempt
	}
	for _, c := range cases {
		if _, err := ParseSpec(c); err == nil {
			t.Errorf("ParseSpec(%q) should have errored", c)
		}
	}
}

func TestSpecString(t *testing.T) {
	s := Spec{LocalPort: 5137, RemoteHost: "localhost", RemotePort: 5137}
	if s.String() != "5137:localhost:5137" {
		t.Errorf("got %q", s.String())
	}
}

func TestStatusString(t *testing.T) {
	cases := map[Status]string{
		StatusStopped:  "stopped",
		StatusStarting: "starting",
		StatusUp:       "up",
		StatusDead:     "dead",
		StatusPortBusy: "port-busy",
		StatusError:    "error",
	}
	for s, want := range cases {
		if s.String() != want {
			t.Errorf("Status(%d).String() = %q, want %q", s, s.String(), want)
		}
	}
}

func TestValidateHost(t *testing.T) {
	good := []string{"host", "user@host", "user@host.example.com", "abc-123_x", "h"}
	for _, h := range good {
		if err := validateHost(h); err != nil {
			t.Errorf("validateHost(%q) unexpected error: %v", h, err)
		}
	}
	bad := []string{
		"",
		" ",
		"-oProxyCommand=foo", // argument injection
		"-N",
		"host;rm -rf /",
		"host with space",
		"host\nnewline",
		"host`backtick`",
		"host$VAR",
	}
	for _, h := range bad {
		if err := validateHost(h); err == nil {
			t.Errorf("validateHost(%q) should have errored", h)
		}
	}
}
