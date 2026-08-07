package server

import (
	"net/http"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/version"
)

// handleGetVersion returns the daemon's build. A dedicated endpoint rather than a
// field on GET /api/status, which already serves an unrelated concern (the Claude
// platform health) — conflating the two would muddy both (#341).
func (s *Server) handleGetVersion(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, api.VersionInfo{
		Version:   version.Version,
		Commit:    version.Commit,
		BuildTime: version.BuildTime,
	})
}
