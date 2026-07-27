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
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
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
		client = http.DefaultClient
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

	var ou oauthUsage
	if err := json.NewDecoder(resp.Body).Decode(&ou); err != nil {
		return nil, fmt.Errorf("decoding usage: %w", err)
	}
	return &api.UsageReport{
		FiveHourPct:   ou.FiveHour.Utilization,
		FiveHourReset: ou.FiveHour.ResetsAt,
		SevenDayPct:   ou.SevenDay.Utilization,
		SevenDayReset: ou.SevenDay.ResetsAt,
		FetchedAt:     now.UTC().Format(time.RFC3339),
	}, nil
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
