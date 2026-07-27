package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCreds(t *testing.T, token string) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"claudeAiOauth":{"accessToken":"` + token + `"}}`
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCreds(t, "tok-123")
	got, err := readToken()
	if err != nil || got != "tok-123" {
		t.Fatalf("readToken = (%q, %v), want tok-123", got, err)
	}
}

func TestFetch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCreds(t, "tok")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("anthropic-beta") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":2.0,"resets_at":"R1"},"seven_day":{"utilization":27.0,"resets_at":"R2"}}`))
	}))
	defer srv.Close()

	f := &Fetcher{Endpoint: srv.URL}
	rep, ok, err := f.Fetch(context.Background(), time.Now())
	if err != nil || !ok {
		t.Fatalf("Fetch: ok=%v err=%v", ok, err)
	}
	if rep.FiveHourPct != 2 || rep.SevenDayPct != 27 || rep.FiveHourReset != "R1" || rep.SevenDayReset != "R2" {
		t.Errorf("report = %+v", rep)
	}
	if rep.FetchedAt == "" {
		t.Error("FetchedAt is empty")
	}
}

func TestBackoffSkipsDuringWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCreds(t, "tok")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &Fetcher{Endpoint: srv.URL}
	now := time.Now()

	if _, ok, err := f.Fetch(context.Background(), now); ok || err == nil {
		t.Fatalf("first fetch should fail: ok=%v err=%v", ok, err)
	}
	// Immediate retry is skipped (still backing off): ok=false, no error.
	if _, ok, err := f.Fetch(context.Background(), now.Add(time.Second)); ok || err != nil {
		t.Errorf("retry during backoff: ok=%v err=%v, want skip", ok, err)
	}
	// After the backoff window it tries again (and fails again).
	if _, _, err := f.Fetch(context.Background(), now.Add(31*time.Second)); err == nil {
		t.Error("retry after backoff should fetch again and fail")
	}
}

func TestBackoffFor(t *testing.T) {
	cases := map[int]time.Duration{
		1:  30 * time.Second,
		2:  60 * time.Second,
		3:  120 * time.Second,
		4:  240 * time.Second,
		5:  300 * time.Second,
		10: 300 * time.Second,
	}
	for failures, want := range cases {
		if got := backoffFor(failures); got != want {
			t.Errorf("backoffFor(%d) = %v, want %v", failures, got, want)
		}
	}
}
