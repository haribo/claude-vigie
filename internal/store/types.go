package store

// Session is the current state of one Claude Code session. Empty strings mean
// "unknown / not set" (e.g. EndedAt is empty until the session ends).
type Session struct {
	ID            string
	Title         string
	User          string // OS account that launched the session
	Machine       string
	ProjectDir    string
	GitBranch     string
	Model         string
	Status        string
	LastTool      string
	Usage         Usage
	StartedAt     string // RFC3339
	LastSeenAt    string // RFC3339
	EndedAt       string // RFC3339, empty while the session is active
	RemoteControl bool   // operator-toggled remote-control flag
	ReportedAt    string // RFC3339 server time of the last report (heartbeat)
	// APIErrorStatus is the HTTP code of a live API error the session hit
	// (500/529/429…), else 0. Set by the watcher; transient — cleared when the
	// session recovers.
	APIErrorStatus int
	// StatusSource is the observer that last set Status: "hook" (event-driven,
	// authoritative for waiting and open turns) or "watch" (poll-derived). It
	// lets reconciliation keep a hook state while letting the watcher retract its
	// own — see docs/design/session-status.md.
	StatusSource string
	// Activity is a short message describing what the session is doing (working)
	// or waiting on (waiting): a tool call or a notification. Cleared on a status
	// change so it never goes stale.
	Activity string
	// StatusChangedAt is when Status last changed (RFC3339, the reporting event's
	// timestamp). It lets a hook `waiting` outlive a stale watcher `working`: the
	// watcher only clears it once the transcript moves past this time.
	StatusChangedAt string
}

// Usage holds cumulative token counters for a session.
type Usage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

// Event is a single entry in the append-only session event log.
type Event struct {
	SessionID string
	Event     string
	Status    string
	CreatedAt string // RFC3339
}
