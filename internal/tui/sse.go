package tui

import (
	"bufio"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/config"
)

// subscribeEvents streams SSE notifications from the server, sending on out for
// each event, and reconnects on failure. The tui's poll covers any gap.
func subscribeEvents(cfg *config.Config, out chan<- struct{}) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/events"
	client := &http.Client{} // no timeout: the stream is long-lived
	for {
		streamEvents(client, url, cfg.Token, out)
		time.Sleep(2 * time.Second) // brief pause before reconnecting
	}
}

func streamEvents(client *http.Client, url, token string, out chan<- struct{}) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data:") {
			select {
			case out <- struct{}{}:
			default:
			}
		}
	}
}
