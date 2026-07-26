package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haribo/claude-fleet/internal/config"
)

func TestConnectionStatuses(t *testing.T) {
	cases := []struct {
		code    int
		wantErr bool
		substr  string
	}{
		{http.StatusOK, false, ""},
		{http.StatusUnauthorized, true, "invalid token"},
		{http.StatusNotFound, true, "not a claude-fleet server"},
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
