// Package api holds the request/response types exchanged between the client
// (reporter, terminal client) and the server. It is shared by both binaries
// and imports nothing else from the project — this is what lets the client
// talk to the server without importing the server or store packages.
package api

// ReportRequest is the payload the reporter POSTs to /api/report for one hook
// event. The server derives the session status from Event.
type ReportRequest struct {
	Event         string `json:"event"`
	SessionID     string `json:"session_id"`
	User          string `json:"user,omitempty"` // OS account that launched the session
	Machine       string `json:"machine"`
	ProjectDir    string `json:"project_dir"`
	GitBranch     string `json:"git_branch,omitempty"`
	Model         string `json:"model,omitempty"`
	Title         string `json:"title,omitempty"` // conversation title (/rename or auto)
	LastTool      string `json:"last_tool,omitempty"`
	Status        string `json:"status,omitempty"`         // explicit status (watcher); empty = derive from event
	RemoteControl *bool  `json:"remote_control,omitempty"` // detected /rc state (watcher); nil = no info
	Usage         *Usage `json:"usage,omitempty"`          // present on Stop / SessionEnd
	Timestamp     string `json:"timestamp"`                // RFC3339, event time
}

// Usage holds token counters.
type Usage struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
}

// SessionView is a session as returned by /api/sessions.
type SessionView struct {
	ID            string  `json:"id"`
	Title         string  `json:"title,omitempty"`
	User          string  `json:"user,omitempty"`
	Machine       string  `json:"machine"`
	ProjectDir    string  `json:"project_dir"`
	GitBranch     string  `json:"git_branch,omitempty"`
	Model         string  `json:"model,omitempty"`
	Status        string  `json:"status"`
	LastTool      string  `json:"last_tool,omitempty"`
	Usage         Usage   `json:"usage"`
	StartedAt     string  `json:"started_at"`
	LastSeenAt    string  `json:"last_seen_at"`
	EndedAt       string  `json:"ended_at,omitempty"`
	RemoteControl bool    `json:"remote_control"`
	Samples       []int64 `json:"samples,omitempty"` // recent output-token samples, oldest first
}

// Settings are the server-wide settings (read/written at /api/settings).
type Settings struct {
	SessionRetention string `json:"session_retention"` // Go duration; "" = disabled
}

// LeaseRequest asks for the usage-fetch lease.
type LeaseRequest struct {
	Holder string `json:"holder"`
}

// LeaseResponse is the reply to a lease request.
type LeaseResponse struct {
	Acquired  bool   `json:"acquired"`
	ExpiresAt string `json:"expires_at"`
}

// UsageReport is the fetched subscription usage the lease holder posts, and
// what GET /api/usage returns. Percentages only, no currency.
type UsageReport struct {
	FiveHourPct   float64 `json:"five_hour_pct"`
	FiveHourReset string  `json:"five_hour_reset,omitempty"`
	SevenDayPct   float64 `json:"seven_day_pct"`
	SevenDayReset string  `json:"seven_day_reset,omitempty"`
	FetchedAt     string  `json:"fetched_at,omitempty"`
}

// WatcherStatus reports when the server last received a watch report, so the
// client can warn that statuses may be stale. LastSeen is empty if never.
type WatcherStatus struct {
	LastSeen string `json:"last_seen,omitempty"` // RFC3339
}

// PlatformStatus is the Claude platform health the server polls from
// status.claude.com and returns at GET /api/status. Indicator mirrors the
// Statuspage scale ("none", "minor", "major", "critical"); it is empty when the
// server has not fetched it yet (or the endpoint is unavailable), in which case
// the client shows nothing. This is a cross-fleet, read-only external signal —
// polled once server-side, never per client (ADR-0005, observe-only).
type PlatformStatus struct {
	Indicator   string `json:"indicator,omitempty"`   // none | minor | major | critical
	Description string `json:"description,omitempty"` // human summary, e.g. "All Systems Operational"
	URL         string `json:"url,omitempty"`         // public status page
	FetchedAt   string `json:"fetched_at,omitempty"`  // RFC3339
}

// DailyStat is one UTC day's aggregated activity for a model. The client sums
// and re-buckets these rows into day/week/month/year/total views.
type DailyStat struct {
	Day            string `json:"day"` // YYYY-MM-DD (UTC)
	Model          string `json:"model"`
	OutputTokens   int64  `json:"output_tokens"`
	WorkingSeconds int64  `json:"working_seconds"`
	WaitingSeconds int64  `json:"waiting_seconds"`
	IdleSeconds    int64  `json:"idle_seconds"`
}

// TopSession is a session ranked by output tokens for the stats view.
type TopSession struct {
	Name         string `json:"name"` // title if set, else the session id (a hash)
	Machine      string `json:"machine"`
	Model        string `json:"model"`
	Status       string `json:"status"`
	OutputTokens int64  `json:"output_tokens"`
}

// StatsResponse is what GET /api/stats returns: daily rollups plus a recent
// top-sessions ranking and the current session count.
type StatsResponse struct {
	Daily        []DailyStat  `json:"daily"`
	TopSessions  []TopSession `json:"top_sessions"`
	SessionCount int          `json:"session_count"`
}
