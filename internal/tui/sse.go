package tui

import (
	"bufio"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/config"
)

// subscribeEvents streams SSE notifications from the server, sending on out for
// each event and reporting connection state on conn (true = streaming, false =
// disconnected). It reconnects on failure; the tui's poll covers any gap.
func subscribeEvents(cfg *config.Config, out chan<- struct{}, conn chan<- bool) {
	url := strings.TrimRight(cfg.ServerURL, "/") + "/api/events"
	client := &http.Client{} // no timeout: the stream is long-lived
	for {
		streamEvents(client, url, cfg.Token, out, conn)
		sendState(conn, false) // disconnected; retry after a brief pause
		time.Sleep(2 * time.Second)
	}
}

func streamEvents(client *http.Client, url, token string, out chan<- struct{}, conn chan<- bool) {
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
	sendState(conn, true) // connected and streaming
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

// sendState reports a connection-state change without blocking; the buffered
// channel keeps the latest state the model has not yet read.
func sendState(conn chan<- bool, live bool) {
	select {
	case conn <- live:
	default:
	}
}
