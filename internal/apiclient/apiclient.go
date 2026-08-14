// Package apiclient performs the client side's authenticated GETs against the
// vigie daemon.
//
// It exists because the same twenty lines — build the URL, bound the request,
// send the bearer token, refuse a non-200, decode the body — were written once
// per endpoint, and twice over for two of them: `/api/watcher` and `/api/version`
// had byte-identical implementations in both `internal/client` (the startup
// preflight) and `internal/tui` (the live model). A fix applied to one would have
// left the other behind in silence (#444).
//
// It is a third package rather than one importing the other because
// `internal/client` already imports `internal/tui`, so the reverse would be a
// cycle. Both are client-side, compiled into the same `vigie` binary, so nothing
// here crosses the deployment boundary of
// [ADR-0003](../../docs/adr/0003-split-client-and-daemon-binaries.md).
package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/haribo/claude-vigie/internal/config"
)

// httpClient carries a timeout — http.DefaultClient has none, so a hung daemon
// would hang the caller forever.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// requestTimeout bounds a single request, inside the client's own ceiling.
const requestTimeout = 5 * time.Second

// Get fetches and decodes a JSON resource from the daemon. `resource` names what
// is being decoded, so a malformed body says which endpoint produced it rather
// than only that some JSON failed.
//
// Any non-200 is an error: this client reads state, and a partial or redirected
// answer is not state.
func Get[T any](cfg *config.Config, path, resource string) (T, error) {
	var zero T
	url := strings.TrimRight(cfg.ServerURL, "/") + path

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("server returned %s", resp.Status)
	}

	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return zero, fmt.Errorf("decoding %s: %w", resource, err)
	}
	return v, nil
}
