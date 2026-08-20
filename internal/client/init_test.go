package client

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haribo/claude-vigie/internal/config"
)

func TestConnectionStatuses(t *testing.T) {
	cases := []struct {
		code    int
		wantErr bool
		substr  string
	}{
		{http.StatusOK, false, ""},
		{http.StatusUnauthorized, true, "invalid token"},
		{http.StatusNotFound, true, "not a vigie server"},
		{http.StatusInternalServerError, true, "unexpected status"},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.code)
		}))
		err := testConnection(&config.Config{ServerURL: srv.URL, Token: "t"})
		srv.Close()

		switch {
		case c.wantErr && err == nil:
			t.Errorf("status %d: want error, got nil", c.code)
		case c.wantErr && !strings.Contains(err.Error(), c.substr):
			t.Errorf("status %d: error %q, want substring %q", c.code, err, c.substr)
		case !c.wantErr && err != nil:
			t.Errorf("status %d: want nil, got %v", c.code, err)
		}
	}
}

// TestInitWritesOnlyTheConfig is the #415 contract: one artifact, one owner. init
// connects the machine; the watcher owns the hooks and the skill and keeps them
// current (ADR-0009). If init installed them again they would age here, and a
// dev-leg run would silently rewrite the production hooks.
func TestInitWritesOnlyTheConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("VIGIE_CONFIG", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// init takes no flags: it asks. The answers are stubbed, not typed.
	withPrompt(t, true, &stubPrompt{answers: map[string]string{
		labelServer: srv.URL,
		labelToken:  "tok",
	}}) // the machine answer is empty, so the hostname default applies

	if code := runInit(nil); code != 0 {
		t.Fatalf("runInit = %d, want 0", code)
	}

	if _, err := os.Stat(filepath.Join(home, ".config", "vigie", "config.toml")); err != nil {
		t.Errorf("the config was not written: %v", err)
	}
	// Nothing under ~/.claude: no hooks, no skill.
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("init touched ~/.claude; the watcher owns those artifacts (err = %v)", err)
	}
}

// TestInitWarnsWhenTheTokenWouldTravelInTheClear is the wiring guard for #581:
// the decision is unit-tested above, this checks `init` actually says it. The
// warning is the whole deliverable — computing it and not printing it is the
// failure mode worth a test.
func TestInitWarnsWhenTheTokenWouldTravelInTheClear(t *testing.T) {
	for _, c := range []struct {
		name     string
		resolves string
		want     bool
	}{
		{name: "a public server warns", resolves: "203.0.113.5", want: true},
		{name: "a private server does not", resolves: "192.168.1.10"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
			t.Setenv("VIGIE_CONFIG", "")

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			// httptest listens on 127.0.0.1, so the address has to come from the
			// resolver stub rather than the URL: `init` must reach a real server
			// *and* believe it lives on a public address.
			original := lookupHost
			lookupHost = func(string) ([]net.IP, error) { return []net.IP{net.ParseIP(c.resolves)}, nil }
			defer func() { lookupHost = original }()
			host := "vigie.test"
			port := srv.URL[strings.LastIndex(srv.URL, ":"):]
			serverURL := "http://" + host + port
			// The probe must still land, so point the transport back at the test
			// server while the URL names something that "resolves" publicly.
			originalClient := httpClient
			httpClient = srv.Client()
			httpClient.Transport = rewriteHost{to: srv.Listener.Addr().String()}
			defer func() { httpClient = originalClient }()

			withPrompt(t, true, &stubPrompt{answers: map[string]string{
				labelServer: serverURL,
				labelToken:  "tok",
			}})

			var code int
			out := captureStderr(t, func() { code = runInit(nil) })
			if code != 0 {
				t.Fatalf("runInit = %d, want 0\n%s", code, out)
			}
			// Compared against what cleartextWarning produces, not against a
			// phrase copied out of it: this guards the wiring, and rewording the
			// warning must not turn it red.
			want := cleartextWarning(serverURL)
			if (want != "") != c.want {
				t.Fatalf("the fixture is wrong: cleartextWarning fired = %v, want %v", want != "", c.want)
			}
			if got := want != "" && strings.Contains(out, want); got != c.want {
				t.Errorf("init printed the warning = %v, want %v\nstderr:\n%s", got, c.want, out)
			}
		})
	}
}

// rewriteHost sends every request to a fixed address, whatever the URL says.
type rewriteHost struct{ to string }

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Host = r.to
	return http.DefaultTransport.RoundTrip(req)
}
