// Package api holds the request/response types exchanged between the client
// (reporter, terminal client) and the server. It is shared by both binaries
// and imports nothing else from the project — this is what lets the client
// talk to the server without importing the server or store packages.
package api

// ReportRequest is the payload the reporter POSTs to /api/report for one hook
// event. The server derives the session status from Event.
type ReportRequest struct {
	Event      string `json:"event"`
	SessionID  string `json:"session_id"`
	User       string `json:"user,omitempty"` // OS account that launched the session
	Machine    string `json:"machine"`
	ProjectDir string `json:"project_dir"`
	GitBranch  string `json:"git_branch,omitempty"`
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"` // reasoning effort of the last assistant turn
	// ContextTokens is the real prompt size of the latest request (#279). A
	// pointer so a known 0 (a just-cleared session, 0%) is distinct from absent
	// (no reading — keep the last known value, render "-"): nil = unknown, &0 =
	// known-empty, &N = known N (#367).
	ContextTokens  *int64 `json:"context_tokens,omitempty"`
	PermissionMode string `json:"permission_mode,omitempty"` // default/acceptEdits/plan/auto/bypassPermissions (#304)
	Title          string `json:"title,omitempty"`           // conversation title (/rename or auto)
	LastTool       string `json:"last_tool,omitempty"`
	// NotificationType is the Claude Code Notification hook's notification_type
	// (e.g. permission_prompt vs idle_prompt); it splits waiting from idle.
	NotificationType string `json:"notification_type,omitempty"`
	// Detail is the contextual detail of the session's current state: the running
	// tool, what a permission prompt is asking about, `shell`, `interrupted`. Named
	// `activity` before #393.
	Detail string `json:"detail,omitempty"`
	// Activity is the pre-#393 name of Detail, still read from a reporter that
	// predates the rename. The hook reporter is deliberately ungated by the version
	// check (docs/design/version-consistency.md), so it can lag the daemon; without
	// this fallback its detail would be dropped silently. Removable once no such
	// client remains.
	Activity       string `json:"activity,omitempty"`
	Status         string `json:"status,omitempty"`           // explicit status (watcher); empty = derive from event
	RemoteControl  *bool  `json:"remote_control,omitempty"`   // detected /rc state (watcher); nil = no info
	RemoteURL      string `json:"remote_url,omitempty"`       // /rc resume URL (watcher); "" clears it, set with RemoteControl
	Usage          *Usage `json:"usage,omitempty"`            // present on Stop / SessionEnd
	APIErrorStatus int    `json:"api_error_status,omitempty"` // HTTP code of a live API error (watcher); 0 = none
	// WatcherVersion/WatcherCommit are the watcher's build, carried on Event=="watch"
	// so the server can track each machine's watcher version (#356).
	WatcherVersion string `json:"watcher_version,omitempty"`
	WatcherCommit  string `json:"watcher_commit,omitempty"`
	// CallMessage rides Event=="call": the message a session raised for the
	// operator (ADR-0010). Optional — a call with no message is still a call, so
	// the event, not this field, is what raises it.
	CallMessage string `json:"call_message,omitempty"`
	Timestamp   string `json:"timestamp"` // RFC3339, event time
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
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	User       string `json:"user,omitempty"`
	Machine    string `json:"machine"`
	ProjectDir string `json:"project_dir"`
	GitBranch  string `json:"git_branch,omitempty"`
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"` // reasoning effort of the last assistant turn
	// ContextTokens is the real prompt size of the latest request (#279). nil when
	// vigie has no reading (rendered "-"); a non-nil value is known, including a
	// known 0 for a just-cleared session shown as 0% (#367).
	ContextTokens *int64 `json:"context_tokens,omitempty"`
	// ContextWindow is the model's window in tokens, and ContextPct how full it
	// is, rounded — both derived by the daemon so no client owns a copy of the
	// model table or of the rounding (ADR-0011, #616). ContextPct is nil exactly
	// when ContextTokens is: a session with a known reading of 0 carries a
	// pointer to 0, which #367 exists to keep distinct from "no reading".
	ContextWindow   int64   `json:"context_window,omitempty"`
	ContextPct      *int    `json:"context_pct,omitempty"`
	PermissionMode  string  `json:"permission_mode,omitempty"` // default/acceptEdits/plan/auto/bypassPermissions (#304)
	Status          string  `json:"status"`
	LastTool        string  `json:"last_tool,omitempty"`
	Usage           Usage   `json:"usage"`
	StartedAt       string  `json:"started_at"`
	LastSeenAt      string  `json:"last_seen_at"`
	EndedAt         string  `json:"ended_at,omitempty"`
	RemoteControl   bool    `json:"remote_control"`
	RemoteURL       string  `json:"remote_url,omitempty"`        // /rc resume URL while remote control is active
	APIErrorStatus  int     `json:"api_error_status,omitempty"`  // HTTP code when Status == "error", else 0
	Detail          string  `json:"detail,omitempty"`            // contextual detail of the current state (#393)
	StatusChangedAt string  `json:"status_changed_at,omitempty"` // RFC3339, when Status last changed
	Samples         []int64 `json:"samples,omitempty"`           // recent output-token samples, oldest first
	// CallAt/CallMessage carry a call the session raised for the operator
	// (ADR-0010). The call is active iff CallAt is non-empty; the message may be
	// empty. Orthogonal to Status — a calling session keeps its own status.
	CallAt      string `json:"call_at,omitempty"`
	CallMessage string `json:"call_message,omitempty"`
}

// Settings are the server-wide settings (read/written at /api/settings).
type Settings struct {
	SessionRetention string `json:"session_retention"` // Go duration; "" = disabled
}

// HeartbeatRequest is a watcher's liveness claim: it is alive on this machine,
// running this build. Liveness is deliberately independent of session reports —
// a watcher with nothing to report is still running
// (docs/design/watcher-liveness.md, #386).
type HeartbeatRequest struct {
	Machine        string `json:"machine"`
	WatcherVersion string `json:"watcher_version,omitempty"`
	WatcherCommit  string `json:"watcher_commit,omitempty"`
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
	// Holder is the machine that fetched this snapshot. The server checks it
	// against the usage lease, so only the machine that acquired the right to
	// fetch can write the figure the whole fleet reads (#515).
	Holder string `json:"holder,omitempty"`
}

// WatcherStatus reports when the server last received a watch report, so the
// client can warn that statuses may be stale. LastSeen is empty if never.
type WatcherStatus struct {
	LastSeen string `json:"last_seen,omitempty"` // RFC3339 — most recent watch report, any machine
	// Machines maps each machine that currently has sessions to the RFC3339 time
	// of its last watch report, or "" when no watcher ever reported for it. Lets
	// the client flag machines running on hooks alone (#284).
	Machines map[string]string `json:"machines,omitempty"`
	// Versions maps each machine to the build its watcher last reported — so an
	// operator can spot a watcher that has drifted behind the daemon (#356). Absent
	// for a machine reporting on hooks alone.
	Versions map[string]VersionInfo `json:"versions,omitempty"`
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

// VersionInfo is the daemon's build, returned at GET /api/version so an operator
// can see which `vigied` they are talking to and spot a client that has drifted
// behind it (#341). Version is "dev" for an untagged build, never empty.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
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
