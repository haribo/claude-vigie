package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"time"

	"github.com/haribo/claude-vigie/internal/clock"

	"github.com/haribo/claude-vigie/internal/api"
)

// DefaultPlatformStatusURL is the Statuspage status endpoint for the Claude
// platform. It needs no auth and is the same for the whole fleet, so the server
// polls it once and fans the result out — never each client (ADR-0005).
const DefaultPlatformStatusURL = "https://status.claude.com/api/v2/status.json"

// PlatformPollInterval is how often the server refreshes the platform status.
// Statuspage changes are coarse; a minute stays well-mannered toward the
// endpoint while remaining timely enough for an outage banner.
const PlatformPollInterval = time.Minute

// platformMetaKey is where the latest platform status snapshot is stored.
const platformMetaKey = "platform_status"

// platformFetchTimeout bounds a single poll.
const platformFetchTimeout = 10 * time.Second

// statusClient is a dedicated client with a timeout (http.DefaultClient has
// none) for polling the public status endpoint.
var statusClient = &http.Client{Timeout: platformFetchTimeout}

func (s *Server) handleGetPlatform(w http.ResponseWriter, r *http.Request) {
	v, ok, err := s.store.GetMeta(r.Context(), platformMetaKey)
	if err != nil {
		s.log.Error("reading platform status", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var ps api.PlatformStatus
	if ok {
		if err := json.Unmarshal([]byte(v), &ps); err != nil {
			s.log.Error("decoding platform status", "error", err)
		}
	}
	s.writeJSON(w, http.StatusOK, ps)
}

// StartPlatformPoller polls url every interval in the background, storing the
// latest platform health and nudging SSE subscribers whenever it changes. It
// returns immediately; the goroutine stops when ctx is done.
func (s *Server) StartPlatformPoller(ctx context.Context, url string, interval time.Duration) {
	go s.platformLoop(ctx, url, interval)
}

func (s *Server) platformLoop(ctx context.Context, url string, interval time.Duration) {
	var lastKey string
	poll := func() {
		cctx, cancel := context.WithTimeout(ctx, platformFetchTimeout)
		defer cancel()
		ps, err := fetchPlatformStatus(cctx, url)
		if err != nil {
			s.log.Warn("polling platform status", "error", err)
			return
		}
		blob, err := json.Marshal(ps)
		if err != nil {
			s.log.Error("encoding platform status", "error", err)
			return
		}
		if err := s.store.SetMeta(ctx, platformMetaKey, string(blob)); err != nil {
			s.log.Error("storing platform status", "error", err)
			return
		}
		// Only nudge clients when the health actually changed, not on every
		// poll (FetchedAt always moves).
		if key := ps.Indicator + "|" + ps.Description; key != lastKey {
			lastKey = key
			s.hub.publish()
		}
	}

	poll()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			poll()
		}
	}
}

// fetchPlatformStatus fetches and parses the Statuspage status.json document.
func fetchPlatformStatus(ctx context.Context, url string) (api.PlatformStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return api.PlatformStatus{}, err
	}
	req.Header.Set("User-Agent", "vigie")

	resp, err := statusClient.Do(req)
	if err != nil {
		return api.PlatformStatus{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return api.PlatformStatus{}, fmt.Errorf("status endpoint returned %s", resp.Status)
	}

	var body struct {
		Page struct {
			URL string `json:"url"`
		} `json:"page"`
		Status struct {
			Indicator   string `json:"indicator"`
			Description string `json:"description"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return api.PlatformStatus{}, fmt.Errorf("decoding platform status: %w", err)
	}
	return api.PlatformStatus{
		Indicator:   body.Status.Indicator,
		Description: body.Status.Description,
		URL:         body.Page.URL,
		FetchedAt:   clock.Now().UTC().Format(time.RFC3339),
	}, nil
}
