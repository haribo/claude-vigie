package apiclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/config"
)

type payload struct {
	Name string `json:"name"`
}

func serve(t *testing.T, h http.HandlerFunc) *config.Config {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &config.Config{ServerURL: srv.URL, Token: "tok"}
}

func TestGetDecodesTheBody(t *testing.T) {
	cfg := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/thing" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q", r.Method)
		}
		_, _ = w.Write([]byte(`{"name":"value"}`))
	})

	got, err := Get[payload](cfg, "/api/thing", "thing")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "value" {
		t.Errorf("name = %q, want value", got.Name)
	}
}

// The token is what every route requires; sending it is the point of having one
// client rather than one per caller.
func TestGetSendsTheBearerToken(t *testing.T) {
	var seen string
	cfg := serve(t, func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := Get[payload](cfg, "/api/thing", "thing"); err != nil {
		t.Fatal(err)
	}
	if seen != "Bearer tok" {
		t.Errorf("Authorization = %q, want \"Bearer tok\"", seen)
	}
}

// A server URL with a trailing slash must not produce a double slash — this is
// operator-supplied configuration, typed by hand.
func TestGetToleratesATrailingSlash(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := &config.Config{ServerURL: srv.URL + "///", Token: "tok"}
	if _, err := Get[payload](cfg, "/api/thing", "thing"); err != nil {
		t.Fatal(err)
	}
	if path != "/api/thing" {
		t.Errorf("path = %q, want /api/thing", path)
	}
}

// Anything but 200 is an error: this client reads state, and a redirect or a
// partial answer is not state. The status must reach the caller, since that is
// what tells an unauthorized token from a dead endpoint.
func TestGetRejectsEveryNon200(t *testing.T) {
	for _, code := range []int{
		http.StatusUnauthorized, http.StatusNotFound,
		http.StatusInternalServerError, http.StatusNoContent, http.StatusAccepted,
	} {
		cfg := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		})
		_, err := Get[payload](cfg, "/api/thing", "thing")
		if err == nil {
			t.Errorf("status %d returned no error", code)
			continue
		}
		if !strings.Contains(err.Error(), http.StatusText(code)) {
			t.Errorf("status %d: error %q does not name the status", code, err)
		}
	}
}

// A malformed body must say which resource produced it — the whole reason Get
// takes a name.
func TestGetNamesTheResourceInADecodeError(t *testing.T) {
	cfg := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":`))
	})

	_, err := Get[payload](cfg, "/api/watcher", "watcher status")
	if err == nil {
		t.Fatal("a truncated body decoded without error")
	}
	if !strings.Contains(err.Error(), "watcher status") {
		t.Errorf("error = %q, want it to name the resource", err)
	}
}

// An unreachable server is an error, not a zero value that would read as "no
// sessions" or "no watcher".
func TestGetFailsOnAnUnreachableServer(t *testing.T) {
	cfg := &config.Config{ServerURL: "http://127.0.0.1:1", Token: "tok"}
	if _, err := Get[payload](cfg, "/api/thing", "thing"); err == nil {
		t.Error("an unreachable server returned no error")
	}
}
