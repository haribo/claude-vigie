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
	Machine    string `json:"machine"`
	ProjectDir string `json:"project_dir"`
	GitBranch  string `json:"git_branch,omitempty"`
	Model      string `json:"model,omitempty"`
	LastTool   string `json:"last_tool,omitempty"`
	Usage      *Usage `json:"usage,omitempty"` // present on Stop / SessionEnd
	Timestamp  string `json:"timestamp"`       // RFC3339, event time
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
	Machine    string `json:"machine"`
	ProjectDir string `json:"project_dir"`
	GitBranch  string `json:"git_branch,omitempty"`
	Model      string `json:"model,omitempty"`
	Status     string `json:"status"`
	LastTool   string `json:"last_tool,omitempty"`
	Usage      Usage  `json:"usage"`
	StartedAt  string `json:"started_at"`
	LastSeenAt string `json:"last_seen_at"`
	EndedAt    string `json:"ended_at,omitempty"`
}
