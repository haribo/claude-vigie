package server

import (
	"net/http"
	"sync"
)

// hub fans out "sessions changed" notifications to SSE subscribers. Each
// subscriber has a buffered channel of size 1, so bursts of changes coalesce
// into a single pending notification.
type hub struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newHub() *hub {
	return &hub{subs: make(map[chan struct{}]struct{})}
}

func (h *hub) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	metricSSESubscribers.Inc()
	return ch
}

func (h *hub) unsubscribe(ch chan struct{}) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
		metricSSESubscribers.Dec()
	}
	h.mu.Unlock()
}

func (h *hub) publish() {
	metricSSEPublished.Inc()
	h.mu.Lock()
	for ch := range h.subs {
		select {
		case ch <- struct{}{}:
		default: // a notification is already pending for this subscriber
		}
	}
	h.mu.Unlock()
}

// handleEvents streams Server-Sent Events: a "sessions" event whenever the
// fleet changes, so clients can refresh without polling.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)

	send := func() bool {
		if _, err := w.Write([]byte("event: sessions\ndata: {}\n\n")); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send() { // initial nudge so the client fetches immediately
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if !send() {
				return
			}
		}
	}
}
