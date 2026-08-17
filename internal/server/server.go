// Package server implements the vigied HTTP API: it accepts session
// event reports, lists sessions, and streams updates over SSE. Only the daemon
// imports this package.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/web"
)

// Store is the persistence surface the server depends on (accept interfaces).
type Store interface {
	UpsertSession(ctx context.Context, s store.Session) error
	GetSession(ctx context.Context, id string) (store.Session, error)
	ApplySession(ctx context.Context, id string, merge func(store.Session, bool) store.Session) (store.Session, error)
	ListSessions(ctx context.Context) ([]store.Session, error)
	AppendEvent(ctx context.Context, e store.Event) error
	LastEvent(ctx context.Context, sessionID string) (store.Event, bool, error)
	AddSample(ctx context.Context, sessionID, at string, outputTokens int64) error
	AddDailyTokens(ctx context.Context, day, model string, delta int64) error
	RaiseTokenMark(ctx context.Context, sessionID string, total int64) (int64, error)
	AddDailyStatusSeconds(ctx context.Context, day, model, status string, secs int64) error
	ListDailyStats(ctx context.Context, sinceDay string) ([]store.DailyStat, error)
	LastSampleAt(ctx context.Context, sessionID string) (string, error)
	ListSamples(ctx context.Context, sessionID, since string, limit int) ([]int64, error)
	AcquireLease(ctx context.Context, holder string, ttl time.Duration, now time.Time) (bool, string, error)
	LeaseHolder(ctx context.Context, now time.Time) (string, bool, error)
	GetMeta(ctx context.Context, key string) (string, bool, error)
	ListMeta(ctx context.Context) (map[string]string, error)
	SetMeta(ctx context.Context, key, value string) error
}

// Server is the HTTP handler set for the fleet API.
type Server struct {
	store Store
	token string
	log   *slog.Logger
	hub   *hub
	clock func() time.Time // injected wall clock; defaults to clock.Now
}

// New returns a Server backed by st, authenticating with token.
func New(st Store, token string, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{store: st, token: token, log: log, hub: newHub(), clock: clock.Now}
}

// now reads the injected clock, falling back to the system clock so a Server
// built as a struct literal (e.g. in tests) still works.
func (s *Server) now() time.Time {
	if s.clock == nil {
		return clock.Now()
	}
	return s.clock()
}

// Handler returns the root HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/report", s.auth(http.HandlerFunc(s.handleReport)))
	mux.Handle("GET /api/sessions", s.auth(http.HandlerFunc(s.handleSessions)))
	mux.Handle("POST /api/usage/lease", s.auth(http.HandlerFunc(s.handleUsageLease)))
	mux.Handle("POST /api/usage", s.auth(http.HandlerFunc(s.handlePostUsage)))
	mux.Handle("GET /api/usage", s.auth(http.HandlerFunc(s.handleGetUsage)))
	mux.Handle("GET /api/events", s.auth(http.HandlerFunc(s.handleEvents)))
	mux.Handle("GET /api/watcher", s.auth(http.HandlerFunc(s.handleWatcher)))
	mux.Handle("POST /api/watcher/heartbeat", s.auth(http.HandlerFunc(s.handleWatcherHeartbeat)))
	mux.Handle("GET /api/status", s.auth(http.HandlerFunc(s.handleGetPlatform)))
	mux.Handle("GET /api/version", s.auth(http.HandlerFunc(s.handleGetVersion)))
	mux.Handle("GET /api/stats", s.auth(http.HandlerFunc(s.handleStats)))
	mux.Handle("GET /api/settings", s.auth(http.HandlerFunc(s.handleGetSettings)))
	mux.Handle("POST /api/settings", s.auth(http.HandlerFunc(s.handleSetSettings)))
	// The read-only web dashboard: the app shell ("/") and its assets ("/static/")
	// are served unauthenticated — they hold no fleet data. The JavaScript then
	// authenticates its own /api calls with the token the operator pastes. "/{$}"
	// matches only the exact root (the app is hash-routed), so unknown paths still
	// 404 instead of serving the shell.
	webUI := web.Handler()
	mux.Handle("GET /{$}", webUI)
	mux.Handle("GET /static/", webUI)
	// RED metrics wrap the whole API; auth stays per-route. /healthz and /metrics
	// live on the ops listener (daemon), never on the token-protected API port.
	// limitBody caps request bodies before any handler reads them.
	return withMetrics(limitBody(mux))
}

// maxBodyBytes bounds a request body; real reports are a few KB, so 1 MiB is
// generous while preventing an unbounded-body memory DoS.
const maxBodyBytes = 1 << 20

// limitBody caps every request body with http.MaxBytesReader. A handler reading
// past the cap gets an *http.MaxBytesError from its decoder, which it can turn
// into a 413 (see handleReport).
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// HealthHandler is the liveness probe, served on the ops listener by the daemon.
func (s *Server) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
}

// auth enforces a constant-time Bearer token check on protected routes.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			s.writeError(w, http.StatusUnauthorized, "invalid or missing token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Error("encoding response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}
