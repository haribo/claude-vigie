package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// #457. A suspended machine's connection dies without a FIN or an RST, so a read
// on it blocks until the OS gives up its keepalive probes — minutes. The
// reconnect loop was correct and never ran, because the function it guards had
// not returned.
//
// The observable condition is not "the network froze", it is "the stream stopped
// saying anything", and that needs no network trickery to reproduce: a server
// that sends one event and then goes quiet is exactly the same thing as far as
// the client can tell.

// silentAfterFirst serves one SSE event, then holds the connection open saying
// nothing until the client gives up.
func silentAfterFirst(t *testing.T) (*httptest.Server, chan struct{}) {
	t.Helper()
	gone := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("no flusher")
			return
		}
		_, _ = fmt.Fprint(w, "event: sessions\ndata: {}\n\n")
		f.Flush()
		<-r.Context().Done() // the client hanging up is what ends this
		select {
		case gone <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(srv.Close)
	return srv, gone
}

// withSilenceLimit shortens the watchdog so a test does not wait 30 s.
func withSilenceLimit(t *testing.T, d time.Duration) {
	t.Helper()
	orig := silenceLimit
	silenceLimit = d
	t.Cleanup(func() { silenceLimit = orig })
}

// The defect: without a watchdog this call never returns, so the reconnect below
// it never happens.
func TestTheStreamGivesUpWhenItStopsHearingAnything(t *testing.T) {
	withSilenceLimit(t, 300*time.Millisecond)
	srv, gone := silentAfterFirst(t)

	out, conn := make(chan struct{}, 4), make(chan bool, 4)
	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		streamEvents(&http.Client{}, srv.URL, "tok", out, conn)
		done <- time.Since(start)
	}()

	select {
	case d := <-done:
		if d > 3*time.Second {
			t.Errorf("the stream took %v to notice silence, want about the limit", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never returned — the reconnect loop can never run")
	}

	select {
	case <-gone:
	case <-time.After(2 * time.Second):
		t.Error("the server never saw the client hang up")
	}
}

// Traffic must keep the stream alive: the watchdog is reset by any line, so an
// active fleet is never disconnected for being busy.
func TestTrafficKeepsTheStreamAlive(t *testing.T) {
	withSilenceLimit(t, 400*time.Millisecond)

	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f, _ := w.(http.Flusher)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-stop:
				return
			case <-tick.C:
				if _, err := fmt.Fprint(w, "event: sessions\ndata: {}\n\n"); err != nil {
					return
				}
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	out, conn := make(chan struct{}, 64), make(chan bool, 8)
	done := make(chan struct{})
	go func() { streamEvents(&http.Client{}, srv.URL, "tok", out, conn); close(done) }()

	select {
	case <-done:
		t.Fatal("a stream delivering events was dropped as silent")
	case <-time.After(1200 * time.Millisecond): // three watchdog windows of traffic
	}
	close(stop)
}

// A keep-alive comment carries no data, so it must not be mistaken for an event —
// but it must still count as the stream being alive.
func TestAKeepAliveCommentIsNotAnEvent(t *testing.T) {
	withSilenceLimit(t, 600*time.Millisecond)

	stop := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f, _ := w.(http.Flusher)
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-stop:
				return // let the handler finish, or srv.Close blocks on it
			case <-tick.C:
				if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
					return
				}
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	out, conn := make(chan struct{}, 64), make(chan bool, 8)
	done := make(chan struct{})
	go func() { streamEvents(&http.Client{}, srv.URL, "tok", out, conn); close(done) }()

	select {
	case <-done:
		t.Fatal("keep-alive comments did not keep the stream alive")
	case <-time.After(900 * time.Millisecond):
	}
	if len(out) != 0 {
		t.Errorf("%d spurious refreshes from comment lines", len(out))
	}
	close(stop)
	<-done // the stream must end once the server does
}

// The indicator must not claim a live stream while every poll is failing: that
// observation was made before the machine went to sleep.
func TestTheGlyphDoesNotClaimLiveWhilePollsFail(t *testing.T) {
	m := stubModel()
	m.sseLive = true
	if got := m.connGlyph(); got == "" {
		t.Fatal("no glyph rendered")
	}
	live := m.connGlyph()

	m.err = errFetch
	if m.connGlyph() == live {
		t.Error("the glyph still claims a live stream while the poll is failing")
	}
}
