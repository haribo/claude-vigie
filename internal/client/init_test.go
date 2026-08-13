package client

import (
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
