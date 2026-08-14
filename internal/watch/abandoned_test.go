package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// #448. Claude Code writes a metadata sidecar next to a conversation —
// custom-title, mode, permission-mode, agent name and color — under the project's
// working directory. A renamed or moved project leaves that sidecar behind, and
// the watcher reported it as a session that never existed: 8 of 148 transcripts on
// one machine hold no turn at all.
//
// The filter cannot be "no conversation". A session you have just started and not
// typed into has none either, and that one must appear — an idle session waiting
// for you is what the dashboard is for. What separates them is Claude Code's own
// session registry, which vigie already reads every scan.

func writeMetadataOnly(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(dir, id+".jsonl"), []string{
		`{"type":"custom-title","customTitle":"renamed","sessionId":"` + id + `"}`,
		`{"type":"permission-mode","permissionMode":"auto","sessionId":"` + id + `"}`,
	})
}

func TestAbandonedMetadataFileIsNotASession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	writeMetadataOnly(t, filepath.Join(root, "proj"), "ghost")

	reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := reportFor(reports, "ghost"); got != nil {
		t.Errorf("an abandoned sidecar was reported as a session: %+v", got)
	}
}

// The case that makes the naive filter wrong: a session started and not typed
// into. It has no turn either, but Claude Code has registered it, so it is live
// and must be shown.
func TestAStartedSessionWithNoTurnIsStillASession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	writeMetadataOnly(t, filepath.Join(root, "proj"), "fresh")
	liveRegistry(t, home, "p.json", "fresh", os.Getpid(), selfStartTime(t))

	reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := reportFor(reports, "fresh"); got == nil {
		t.Error("a live session with no turn yet disappeared from the dashboard")
	}
}

// A finished session is not in the registry any more, but it held a conversation
// and must still be reported — otherwise every ended session would vanish.
func TestAnEndedConversationIsStillASession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(proj, "done.jsonl"), []string{
		`{"sessionId":"done","cwd":"/w","type":"assistant","message":{"id":"m1","model":"claude-opus-4-8","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}}`,
	})

	reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := reportFor(reports, "done"); got == nil {
		t.Error("an ended session with a conversation was dropped")
	}
}

// A user line alone is a conversation: the operator typed something, even if
// Claude has not answered yet.
func TestAUserLineAloneCountsAsAConversation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	writeLines(t, filepath.Join(proj, "typed.jsonl"), []string{
		`{"sessionId":"typed","cwd":"/w","type":"user","message":{"role":"user","content":"hello"}}`,
	})

	reports, err := newScanner().scan(root, "m", 24*time.Hour, time.Now())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := reportFor(reports, "typed"); got == nil {
		t.Error("a session with a typed prompt and no answer yet was dropped")
	}
}
