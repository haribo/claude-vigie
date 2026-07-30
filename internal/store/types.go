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
