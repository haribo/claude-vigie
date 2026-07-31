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

	"github.com/haribo/claude-fleet/internal/clock"
	"github.com/haribo/claude-fleet/internal/server"
	"github.com/haribo/claude-fleet/internal/store"
	"github.com/haribo/claude-fleet/internal/version"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "address the server listens on")
	dbPath := fs.String("db", "claude-fleet.db", "path to the SQLite database file")
	tokenFlag := fs.String("token", "", "shared auth token (else $FLEET_TOKEN, else auto-generated)")
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

	token, err := resolveToken(context.Background(), st, *tokenFlag)
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

	srv := &http.Server{
		Handler:           srvInst.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

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

	log.Info("claude-fleetd listening", "addr", *addr, "db", *dbPath)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serving", "error", err)
		return 1
	}
	<-idle
	return 0
}

// startOpsListener serves /healthz and /metrics on a separate address, so the
// main API port carries only the token-protected API.
func startOpsListener(addr string, srv *server.Server, log *slog.Logger) {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", srv.HealthHandler())
	mux.Handle("GET /metrics", server.MetricsHandler())
	ops := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
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
func pruneLoop(st *store.Store, defaultRetention time.Duration, log *slog.Logger) {
	ctx := context.Background()
	if v, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey); !ok || v == "" {
		_ = st.SetMeta(ctx, server.RetentionMetaKey, defaultRetention.String())
	}
	prune := func() {
		retention := defaultRetention
		if v, ok, _ := st.GetMeta(ctx, server.RetentionMetaKey); ok {
			if v == "" {
				return // disabled
			}
			if d, err := time.ParseDuration(v); err == nil {
				retention = d
			}
		}
		if retention <= 0 {
			return
		}
		n, err := st.PruneSessions(ctx, retention, clock.Now())
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

// resolveToken returns the auth token: the flag, else $FLEET_TOKEN, else the
// value persisted in the store, else a freshly generated one (persisted and
// printed so the operator can share it).
func resolveToken(ctx context.Context, st *store.Store, flagToken string) (string, error) {
	if flagToken != "" {
		return flagToken, nil
	}
	if env := os.Getenv("FLEET_TOKEN"); env != "" {
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
	fmt.Fprintf(os.Stderr, "generated fleet token: %s\n", token)
	return token, nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
