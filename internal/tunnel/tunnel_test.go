package tunnel

import (
	"net"
	"strconv"
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
	cases := []string{"", "abc", "5137:host", "0", "99999", ":host:80", "80:host:abc"}
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

// pickFreePort returns a port that was free at the moment of the call.
func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(p)
	return port
}

func TestStartDetectsPortBusy(t *testing.T) {
	// Hold a port so Start() must report it busy.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(p)

	tn := New("nonexistent.invalid", Spec{LocalPort: port, RemoteHost: "localhost", RemotePort: port})
	tn.Start()
	defer tn.Stop()

	status, _ := tn.Snapshot()
	if status != StatusPortBusy {
		t.Errorf("status = %v, want StatusPortBusy", status)
	}
}

func TestStopOnUnstartedIsSafe(t *testing.T) {
	tn := New("h", Spec{LocalPort: pickFreePort(t)})
	tn.Stop()
	tn.Stop()
	status, _ := tn.Snapshot()
	if status != StatusStopped {
		t.Errorf("status = %v, want StatusStopped", status)
	}
}

func TestStatusString(t *testing.T) {
	cases := map[Status]string{
		StatusStopped:  "stopped",
		StatusStarting: "starting",
		StatusUp:       "up",
		StatusPortBusy: "port-busy",
		StatusError:    "error",
	}
	for s, want := range cases {
		if s.String() != want {
			t.Errorf("Status(%d).String() = %q, want %q", s, s.String(), want)
		}
	}
}
