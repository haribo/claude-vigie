package watch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteControlled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("1.json", `{"sessionId":"s-on","bridgeSessionId":"session_x"}`)
	write("2.json", `{"sessionId":"s-off","bridgeSessionId":""}`)
	write("3.json", `{"sessionId":"s-none"}`) // no bridge field
	write("bad.txt", `not a session file`)    // ignored (wrong extension)

	m := remoteControlled()
	if !m["s-on"] {
		t.Error("s-on has a bridge → should be rc")
	}
	if m["s-off"] {
		t.Error("s-off has empty bridge → should not be rc")
	}
	if m["s-none"] {
		t.Error("s-none has no bridge → should not be rc")
	}
}
