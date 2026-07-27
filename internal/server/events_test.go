package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHubCoalesces(t *testing.T) {
	h := newHub()
	ch := h.subscribe()
	defer h.unsubscribe(ch)

	h.publish()
	select {
	case <-ch:
	default:
		t.Fatal("subscriber not notified after publish")
	}

	// Two publishes without a read coalesce into a single pending notification.
	h.publish()
	h.publish()
	select {
	case <-ch:
	default:
		t.Fatal("expected a pending notification")
	}
	select {
	case <-ch:
		t.Fatal("expected coalesced notifications (only one)")
	default:
	}
}

func TestEventsEndpointStreams(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content-type = %q, want text/event-stream", ct)
	}
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event: sessions") {
		t.Errorf("initial event = %q, want an 'event: sessions' line", string(buf[:n]))
	}
}
