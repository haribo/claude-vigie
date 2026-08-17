package daemon

import (
	"net/http"
	"testing"
	"time"
)

// #560. Both listeners set ReadHeaderTimeout and nothing else. With IdleTimeout
// unset Go falls back to ReadTimeout, which is also unset, so keep-alive
// connections are never closed.
//
// The second assertion is the load-bearing one, and it is why these servers are
// built by a function at all: WriteTimeout must stay zero. It bounds the whole
// response, and `GET /api/events` is an SSE stream held open for as long as a
// dashboard is watching — setting it would sever every client on a fixed
// cadence. A reader who sees three timeouts set and one missing will "fix" it
// unless something says otherwise. This is that something.
func TestBothListenersBoundIdleConnections(t *testing.T) {
	for name, srv := range map[string]*http.Server{
		"api": apiServer(http.NewServeMux()),
		"ops": opsServer("127.0.0.1:0", http.NewServeMux()),
	} {
		if srv.ReadHeaderTimeout == 0 {
			t.Errorf("%s: ReadHeaderTimeout is unset", name)
		}
		if srv.IdleTimeout == 0 {
			t.Errorf("%s: IdleTimeout is unset — a quiet keep-alive connection is never closed", name)
		}
		if srv.WriteTimeout != 0 {
			t.Errorf("%s: WriteTimeout is %v, want unset — it would cut the SSE stream at /api/events",
				name, srv.WriteTimeout)
		}
	}
}

func TestTheIdleTimeoutOutlastsTheSSEHeartbeat(t *testing.T) {
	// The server beats every 10 s on an idle stream (internal/server/events.go).
	// IdleTimeout applies between requests rather than during one, so this is not
	// a correctness dependency — but a value below the heartbeat would be a
	// standing invitation to confuse the two.
	if got := apiServer(http.NewServeMux()).IdleTimeout; got < 30*time.Second {
		t.Errorf("IdleTimeout is %v — too close to the 10 s SSE heartbeat to read as unrelated", got)
	}
}
