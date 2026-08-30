package watch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/config"
	"github.com/haribo/claude-vigie/internal/usage"
)

// #646. The client half. A machine takes the usage lease before it knows it can
// fetch, and one that then fetches nothing used to keep renewing it every cycle —
// so nobody else ever fetched and the gauges stayed empty for the whole fleet.
//
// The lease is a right to fetch, not a right to hold (docs/design/usage.md § 2):
// every way out of the cycle that is not a successful post hands it back.
func TestACycleThatFetchesNothingReleasesTheLease(t *testing.T) {
	var mu sync.Mutex
	var released bool
	var posted int

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/api/usage/lease":
			var req api.LeaseRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Release {
				released = true
				_, _ = w.Write([]byte(`{}`))
				return
			}
			_, _ = w.Write([]byte(`{"acquired":true,"expires_at":"2026-08-30T12:12:00Z"}`))
		case "/api/usage":
			posted++
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer daemon.Close()

	// A usage endpoint that refuses, which is what a machine with no local
	// credentials amounts to from here: the lease is taken and nothing comes back.
	refuses := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer refuses.Close()

	cfg := &config.Config{ServerURL: daemon.URL, Token: "t", Machine: "m1"}
	usageCycle(context.Background(), cfg, &usage.Fetcher{Endpoint: refuses.URL, Client: daemon.Client()})

	mu.Lock()
	defer mu.Unlock()
	if posted != 0 {
		t.Fatalf("the cycle posted %d usage reports after a failed fetch", posted)
	}
	if !released {
		t.Error("the cycle kept the lease after fetching nothing — every other machine stays locked out")
	}
}
