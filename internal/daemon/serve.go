package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/server"
	"github.com/haribo/claude-vigie/internal/store"
	"github.com/haribo/claude-vigie/internal/version"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "address the server listens on (bind a reachable interface, e.g. :8080, for cross-machine clients)")
	dbPath := fs.String("db", "vigie.db", "path to the SQLite database file")
	retention := fs.Duration("session-retention", 24*time.Hour, "delete sessions not reported within this window (0 disables)")
	metricsAddr := fs.String("metrics-addr", "127.0.0.1:9464", "ops listener for /metrics and /healthz (empty disables)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Error("opening store", "error", err)
		return 1
	}
	defer func() { _ = st.Close() }()

	token, err := resolveToken(context.Background(), st)
	if err != nil {
		log.Error("resolving token", "error", err)
		return 1
	}

	// Bind up front so a failure (e.g. port already in use) is reported before
	// we log "listening".
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Error("cannot bind address", "addr", *addr, "error", err)
		return 1
	}

	srvInst := server.New(st, token, log)
	// Poll the Claude platform status once for the whole fleet and fan it out
	// over SSE (observe-only external signal). The goroutine stops on exit.
	srvInst.StartPlatformPoller(context.Background(), server.DefaultPlatformStatusURL, server.PlatformPollInterval)

	// Operational metrics on a separate listener (localhost by default): the API
	// port stays the token-protected API only; /healthz and /metrics live here.
	server.SetBuildInfo(version.Version, runtime.Version())
	server.RegisterStateCollector(st, *dbPath)
	if *metricsAddr != "" {
		startOpsListener(*metricsAddr, srvInst, log)
	}

	srv := apiServer(srvInst.Handler())

	idle := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Error("shutdown", "error", err)
		}
		close(idle)
	}()

	if *retention > 0 {
		go pruneLoop(st, *retention, log)
	}

	log.Info("vigied listening", "addr", *addr, "db", *dbPath)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serving", "error", err)
		return 1
	}
	<-idle
	return 0
}

// The timeouts both listeners carry.
//
// **WriteTimeout is deliberately absent, and must stay absent**: it bounds the
// whole response, and `GET /api/events` is a Server-Sent Events stream that stays
// open for as long as the client is watching. Setting it would cut every
// dashboard's stream on a fixed cadence, which is not a timeout, it is a bug.
//
// idleTimeout was absent too, and that one was an oversight: unset, Go falls back
// to ReadTimeout, which is also unset, so a keep-alive connection is never closed
// and a client that opens sockets and goes quiet holds them forever. Dormant
// while the bind is 127.0.0.1 — the default — and not dormant in the
// cross-machine deployment `deployment.md` documents (#560).
//
// They are set in each literal rather than by a shared helper so gosec can see
// ReadHeaderTimeout is configured; the constants are what keeps the two in step.
const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 120 * time.Second
)

// apiServer is the token-protected API listener.
func apiServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: readHeaderTimeout, IdleTimeout: idleTimeout}
}

// opsServer is the unauthenticated /healthz and /metrics listener.
func opsServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: readHeaderTimeout, IdleTimeout: idleTimeout}
}

// startOpsListener serves /healthz and /metrics on a separate address, so the
// main API port carries only the token-protected API.
func startOpsListener(addr string, srv *server.Server, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", srv.HealthHandler())
	mux.Handle("GET /metrics", server.MetricsHandler())
	ops := opsServer(addr, mux)
	go func() {
		log.Info("ops listener", "addr", addr)
		if err := ops.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("ops listener", "error", err)
		}
	}()
}

// pruneInterval is how often stale sessions are garbage-collected.
const pruneInterval = time.Hour

// pruneLoop garbage-collects sessions older than the configured retention, on
// start and then hourly, so the database stays bounded. The retention lives in
// a meta key (editable via /api/settings); the flag only seeds it on first run.
// retention is what pruneLoop decides before touching the database: whether to
// delete anything at all, and with what window.
//
// It is a pure function of the stored setting so the decision can be tested
// without running a ticker — the branches below all end in rows being deleted, or
// not, and every one of them used to be silent (#442).
type retention struct {
	window time.Duration
	prune  bool
	// warning is set when a stored value could not be used and the default was
	// applied instead. Falling back is the long-standing behavior; saying nothing
	// about it was not.
	warning string
}

func decideRetention(stored string, storedOK bool, def time.Duration) retention {
	window := def
	switch {
	case !storedOK:
		// Nothing stored yet: the default governs.
	case stored == "":
		return retention{} // explicitly disabled by the operator
	default:
		d, err := time.ParseDuration(stored)
		if err != nil {
			return retention{window: def, prune: def > 0,
				warning: fmt.Sprintf("retention setting %q is not a duration; using %s", stored, def)}
		}
		window = d
	}
	if window <= 0 {
		return retention{} // a non-positive window disables pruning
	}
	return retention{window: window, prune: true}
}

// seedRetention writes the default retention on first run, so the settings API
// has something to show rather than an empty row.
//
// It fires only on an *absent* key. An empty stored value is not a blank to be
// filled: it is `off (keep all)`, the choice Settings offers, and treating the
// two alike meant every daemon restart wrote 24 h over the operator's decision
// and pruned the sessions it was meant to keep (#656). `GetMeta` separates them
// — absent is `ok=false`, empty is `ok=true` — and `decideRetention` already
// reads the distinction correctly; only this write erased it.
//
// A non-positive default has nothing to seed: an absent key already means "the
// default governs", and decideRetention answers `no pruning` for it.
func seedRetention(ctx context.Context, st *store.Store, def time.Duration) {
	if def <= 0 {
		return
	}
	if _, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey); !ok {
		_ = st.SetMeta(ctx, server.RetentionMetaKey, def.String())
	}
}

func pruneLoop(st *store.Store, defaultRetention time.Duration, log *slog.Logger) {
	ctx := context.Background()
	seedRetention(ctx, st, defaultRetention)
	lastWarning := ""
	prune := func() {
		v, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey)
		r := decideRetention(v, ok, defaultRetention)
		if r.warning != "" && r.warning != lastWarning {
			log.Warn("retention", "problem", r.warning) // announce a transition only
		}
		lastWarning = r.warning
		if !r.prune {
			return
		}
		n, err := st.PruneSessions(ctx, r.window, clock.Now())
		if err != nil {
			log.Error("pruning sessions", "error", err)
		} else if n > 0 {
			log.Info("pruned old sessions", "count", n)
			server.IncPruned(n)
		}
	}
	prune()
	t := time.NewTicker(pruneInterval)
	defer t.Stop()
	for range t.C {
		prune()
	}
}

// tokenEnv is the one way to supply a chosen token. There is no flag: a token on
// the command line is published to every local user through /proc/PID/cmdline,
// which is world-readable, while /proc/PID/environ is readable only by the owner.
// Two ways to pass a secret, where the more discoverable one leaks it, is a trap
// rather than a convenience (#465).
const tokenEnv = "VIGIE_TOKEN"

// resolveToken returns the auth token: $VIGIE_TOKEN, else the value persisted in
// the store, else a freshly generated one (persisted and printed so the operator
// can share it).
//
// The environment wins over the stored value on purpose: a token the operator set
// explicitly should beat one the daemon persisted for itself.
func resolveToken(ctx context.Context, st *store.Store) (string, error) {
	if env := os.Getenv(tokenEnv); env != "" {
		return env, nil
	}
	if v, ok, err := st.GetMeta(ctx, "token"); err != nil {
		return "", err
	} else if ok {
		return v, nil
	}

	token, err := generateToken()
	if err != nil {
		return "", err
	}
	if err := st.SetMeta(ctx, "token", token); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "generated vigie token: %s\n", token)
	return token, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
