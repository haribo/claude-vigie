package tui

import (
	"bufio"
	"context"
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

// silenceLimit is how long the client waits to hear anything at all before it
// treats the stream as dead. The server sends a keep-alive comment every 10 s
// (internal/server/events.go), so this is three missed beats.
//
// It exists because a suspended machine's connection dies without a FIN or an
// RST: the socket stays open as far as the process is concerned, and a read on it
// blocks until the OS gives up on its keepalive probes — minutes. The reconnect
// loop below was correct all along and simply never ran, because the function it
// guards had not returned (#457).
var silenceLimit = 30 * time.Second

func streamEvents(client *http.Client, url, token string, out chan<- struct{}, conn chan<- bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A watchdog, reset by every line the stream delivers. Canceling the request
	// unblocks the read below, which is the only thing that can: a Scanner has no
	// deadline of its own.
	watchdog := time.AfterFunc(silenceLimit, cancel)
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		watchdog.Reset(silenceLimit) // any line, comment included, proves it is alive
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
