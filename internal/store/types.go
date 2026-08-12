package store

// Session is the current state of one Claude Code session. Empty strings mean
// "unknown / not set" (e.g. EndedAt is empty until the session ends).
type Session struct {
	ID         string
	Title      string
	User       string // OS account that launched the session
	Machine    string
	ProjectDir string
	GitBranch  string
	Model      string
	// Effort is the reasoning effort of the last assistant turn (low/medium/high/
	// xhigh/max), or empty when unknown. Derived from the transcript, best-effort.
	Effort string
	// ContextTokens is the real prompt size of the latest request (input +
	// cache-read + cache-creation tokens), for the context-fill %. Read together
	// with ContextKnown: 0 is known-empty (a just-cleared session, shown 0%) only
	// when ContextKnown is true, else it is unknown (#367).
	ContextTokens int64
	// ContextKnown reports whether ContextTokens is a real reading. False means
	// vigie has no context reading for the session (rendered "-"); true means the
	// count is known, including a known 0 (0%). Set by the watcher, which parses
	// the transcript on every scan (#367).
	ContextKnown bool
	// PermissionMode is the session's permission mode
	// (default/acceptEdits/plan/auto/bypassPermissions), empty when unknown (#304).
	PermissionMode string
	Status         string
	LastTool       string
	Usage          Usage
	StartedAt      string // RFC3339
	LastSeenAt     string // RFC3339
	EndedAt        string // RFC3339, empty while the session is active
	RemoteControl  bool   // detected /rc state, read-only (ADR-0005/0007)
	RemoteURL      string // /rc resume URL (https://claude.ai/code/session_…) while active
	ReportedAt     string // RFC3339 server time of the last report (heartbeat)
	// APIErrorStatus is the HTTP code of a live API error the session hit
	// (500/529/429…), else 0. Set by the watcher; transient — cleared when the
	// session recovers.
	APIErrorStatus int
	// StatusSource is the observer that last set Status: "hook" (event-driven,
	// authoritative for waiting and open turns) or "watch" (poll-derived). It
	// lets reconciliation keep a hook state while letting the watcher retract its
	// own — see docs/design/session-status.md.
	StatusSource string
	// Detail is the contextual detail of the current state: the running tool, what
	// a permission prompt is asking about, `shell`, `interrupted`, a call message.
	// Cleared on a status change so it never goes stale. Persisted in the column
	// still named `activity`: renaming a stored column for cosmetics would be risk
	// without benefit (#393).
	Detail string
	// StatusChangedAt is when Status last changed (RFC3339, the reporting event's
	// timestamp). It lets a hook `waiting` outlive a stale watcher `working`: the
	// watcher only clears it once the transcript moves past this time.
	StatusChangedAt string
	// CallMessage and CallAt hold a call the session raised for the operator
	// (ADR-0010): the message (may be empty — a call with no message is still a
	// call) and when it was raised (RFC3339). A call is active iff CallAt is
	// non-empty. Orthogonal to Status: a calling session keeps its own status. Set
	// by the session and cleared by it resuming work (UserPromptSubmit) or ending
	// (SessionEnd) — never by an action on vigie (ADR-0007).
	CallMessage string
	CallAt      string
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
