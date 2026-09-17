// Package usage fetches Claude subscription usage from the (unofficial) OAuth
// usage endpoint using the local OAuth credentials. The token never leaves the
// machine; only percentages and reset times are returned for reporting.
// Client-side.
package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
)

const (
	defaultEndpoint = "https://api.anthropic.com/api/oauth/usage"
	betaHeader      = "oauth-2025-04-20"
	baseBackoff     = 30 * time.Second
	maxBackoff      = 300 * time.Second
)

func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

// readToken reads the current OAuth access token. It is read fresh on every
// fetch so a token refreshed by Claude Code is picked up.
func readToken() (string, error) {
	path, err := credentialsPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading credentials: %w", err)
	}
	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("parsing credentials: %w", err)
	}
	if creds.ClaudeAiOauth.AccessToken == "" {
		return "", errors.New("no oauth access token in credentials")
	}
	return creds.ClaudeAiOauth.AccessToken, nil
}

type oauthUsage struct {
	FiveHour struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"five_hour"`
	SevenDay struct {
		Utilization float64 `json:"utilization"`
		ResetsAt    string  `json:"resets_at"`
	} `json:"seven_day"`
	// Limits is the endpoint's own list of what is being enforced. Each entry
	// declares what it applies to, which is why the model-scoped limit is read from
	// here rather than from a field named after a model: the payload still carries
	// flat `seven_day_opus` / `seven_day_sonnet` keys and both are null, so that
	// shape was abandoned upstream. Reviving it would be the hand-kept list #821 is
	// about — the next model to carry a limit would be invisible in turn (#840).
	Limits []oauthLimit `json:"limits"`
}

// oauthLimit is one entry of that list. Only the fields vigie renders are parsed;
// the payload carries more, including figures in the operator's currency that have
// no business leaving this machine.
type oauthLimit struct {
	Kind     string  `json:"kind"` // session | weekly_all | weekly_scoped
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resets_at"`
	Scope    *struct {
		Model *struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

// scopedLimit returns the weekly limit bound to a single model, or nil when none
// is in force.
//
// Keyed on the entry's own `kind` and on it actually naming a model: an entry that
// says it is scoped but names nothing is not something to draw a gauge for, and
// inventing a label would put a second vocabulary beside Claude's.
func (o oauthUsage) scopedLimit() *api.ScopedLimit {
	for _, l := range o.Limits {
		if l.Kind != "weekly_scoped" || l.Scope == nil || l.Scope.Model == nil {
			continue
		}
		if l.Scope.Model.DisplayName == "" {
			continue
		}
		return &api.ScopedLimit{
			Label: l.Scope.Model.DisplayName,
			Pct:   l.Percent,
			Reset: l.ResetsAt,
		}
	}
	return nil
}

// parseUsage decodes the endpoint's body into the report the fleet shares. Split
// from the request so the shape can be tested against a recorded payload without a
// round trip — the transport is not what this has ever got wrong.
func parseUsage(body []byte) (*api.UsageReport, error) {
	var ou oauthUsage
	if err := json.Unmarshal(body, &ou); err != nil {
		return nil, fmt.Errorf("decoding usage: %w", err)
	}
	return &api.UsageReport{
		FiveHourPct:   ou.FiveHour.Utilization,
		FiveHourReset: ou.FiveHour.ResetsAt,
		SevenDayPct:   ou.SevenDay.Utilization,
		SevenDayReset: ou.SevenDay.ResetsAt,
		Scoped:        ou.scopedLimit(),
	}, nil
}

// Fetcher fetches usage with exponential backoff on failure, acting as a
// circuit breaker against the aggressively rate-limited endpoint.
type Fetcher struct {
	Endpoint string
	Client   *http.Client
	failures int
	nextTry  time.Time
}

// Fetch returns the current usage, or ok=false when it is backing off or the
// call fails. now drives the backoff clock.
func (f *Fetcher) Fetch(ctx context.Context, now time.Time) (*api.UsageReport, bool, error) {
	if now.Before(f.nextTry) {
		return nil, false, nil
	}
	rep, err := f.do(ctx, now)
	if err != nil {
		f.failures++
		f.nextTry = now.Add(backoffFor(f.failures))
		return nil, false, err
	}
	f.failures = 0
	return rep, true, nil
}

// defaultHTTPClient is the fallback when no client is injected; it carries a
// timeout (http.DefaultClient has none).
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (f *Fetcher) do(ctx context.Context, now time.Time) (*api.UsageReport, error) {
	token, err := readToken()
	if err != nil {
		return nil, err
	}
	endpoint := f.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	client := f.Client
	if client == nil {
		client = defaultHTTPClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", betaHeader)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching usage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage endpoint returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading usage: %w", err)
	}
	rep, err := parseUsage(body)
	if err != nil {
		return nil, err
	}
	rep.FetchedAt = now.UTC().Format(time.RFC3339)
	return rep, nil
}

// backoffFor returns 30, 60, 120, 240, 300 (capped) for successive failures.
func backoffFor(failures int) time.Duration {
	d := baseBackoff
	for i := 1; i < failures; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	return d
}
