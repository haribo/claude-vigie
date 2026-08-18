package report

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/reachability"
)

// hangingDaemon is a server that accepts the connection and never answers — the
// failure that costs (docs/design/unreachable-daemon.md § 1). A daemon merely
// stopped answers `connection refused` instantly and would make every assertion
// below pass against the unfixed code.
func hangingDaemon(t *testing.T) (url string, connections func() int) {
	t.Helper()
	release := make(chan struct{})
	var mu sync.Mutex
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		<-release
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv.URL, func() int {
		mu.Lock()
		defer mu.Unlock()
		return hits
	}
}

// TestTheHookStopsPostingToAnUnreachableDaemon guards #578.
//
// Measured before the fix: three PostToolUse events against a daemon that never
// answers cost 3.00 s each — 9.01 s and three connections, because nothing was
// remembered between runs. A turn doing 80 tool calls lost about four minutes,
// and the operator was told nothing.
//
// The assertion is the connection count, not the elapsed time: a wall-clock
// bound is what a loaded CI runner makes flaky, while "opened no connection" is
// the invariant the design states and cannot pass by accident.
func TestTheHookStopsPostingToAnUnreachableDaemon(t *testing.T) {
	url, connections := hangingDaemon(t)
	writeConfig(t, url)
	shortenPostTimeout(t)

	const payload = `{"session_id":"s1","cwd":"/p","hook_event_name":"PostToolUse","tool_name":"Bash"}`
	for i := range 3 {
		if err := Run("PostToolUse", strings.NewReader(payload)); err == nil {
			t.Fatalf("report %d: want an error against a daemon that never answers", i+1)
		}
	}

	if got := connections(); got != 1 {
		t.Errorf("the daemon received %d connections, want 1 — only the first report may pay the deadline", got)
	}
}

// TestTheHookPostsAgainOnceTheMarkGoesStale: the mark is a pause, not a
// shutdown. Nothing clears it on recovery, so the window expiring is what makes
// the arrangement self-healing (§ 4).
func TestTheHookPostsAgainOnceTheMarkGoesStale(t *testing.T) {
	url, connections := hangingDaemon(t)
	writeConfig(t, url)
	shortenPostTimeout(t)

	const payload = `{"session_id":"s2","cwd":"/p","hook_event_name":"PostToolUse","tool_name":"Bash"}`
	_ = Run("PostToolUse", strings.NewReader(payload))
	_ = Run("PostToolUse", strings.NewReader(payload))
	if got := connections(); got != 1 {
		t.Fatalf("inside the window the daemon received %d connections, want 1", got)
	}

	ageTheUnreachableMark(t, url)

	_ = Run("PostToolUse", strings.NewReader(payload))
	if got := connections(); got != 2 {
		t.Errorf("after the window the daemon received %d connections, want 2 — an expired mark must re-probe", got)
	}
}

// TestAnAnsweringDaemonIsNeverMarkedUnreachable: an HTTP error is an answer. The
// daemon is reachable and refused the report for its content, which must not
// suppress the next one (§ 2).
func TestAnAnsweringDaemonIsNeverMarkedUnreachable(t *testing.T) {
	var mu sync.Mutex
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusConflict)
	}))
	t.Cleanup(srv.Close)
	writeConfig(t, srv.URL)
	shortenPostTimeout(t)

	const payload = `{"session_id":"s3","cwd":"/p","hook_event_name":"PostToolUse","tool_name":"Bash"}`
	_ = Run("PostToolUse", strings.NewReader(payload))
	_ = Run("PostToolUse", strings.NewReader(payload))

	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Errorf("the daemon received %d reports, want 2 — a refusal is not unreachability", hits)
	}
}

// ageTheUnreachableMark pushes the mark past its window, so a test can reach the
// other side of it without sleeping through StaleAfter.
func ageTheUnreachableMark(t *testing.T, serverURL string) {
	t.Helper()
	stale := clock.Now().Add(-reachability.StaleAfter - time.Second)
	if err := reachability.Mark(serverURL, stale, errors.New("aged by the test")); err != nil {
		t.Fatal(err)
	}
}

func shortenPostTimeout(t *testing.T) {
	t.Helper()
	original := postTimeout
	postTimeout = 200 * time.Millisecond
	t.Cleanup(func() { postTimeout = original })
}
