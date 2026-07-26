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
	"syscall"
	"time"

	"github.com/haribo/claude-fleet/internal/server"
	"github.com/haribo/claude-fleet/internal/store"
)

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "address the server listens on")
	dbPath := fs.String("db", "claude-fleet.db", "path to the SQLite database file")
	tokenFlag := fs.String("token", "", "shared auth token (else $FLEET_TOKEN, else auto-generated)")
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

	srv := &http.Server{
		Handler:           server.New(st, token, log).Handler(),
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

	log.Info("claude-fleetd listening", "addr", *addr, "db", *dbPath)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serving", "error", err)
		return 1
	}
	<-idle
	return 0
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
