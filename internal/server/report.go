package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/haribo/claude-fleet/internal/api"
	"github.com/haribo/claude-fleet/internal/store"
)

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var req api.ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.SessionID == "" || req.Event == "" {
		s.writeError(w, http.StatusBadRequest, "session_id and event are required")
		return
	}

	ctx := r.Context()
	existing, err := s.store.GetSession(ctx, req.SessionID)
	isNew := errors.Is(err, store.ErrNotFound)
	if err != nil && !isNew {
		s.log.Error("loading session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	sess := applyReport(existing, isNew, req)
	if err := s.store.UpsertSession(ctx, sess); err != nil {
		s.log.Error("upserting session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.store.AppendEvent(ctx, store.Event{
		SessionID: sess.ID, Event: req.Event, Status: sess.Status, CreatedAt: req.Timestamp,
	}); err != nil {
		// The session is already updated; the event log is best-effort.
		s.log.Error("appending event", "error", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// applyReport merges a report into the session state (read-modify-write).
// Fields absent from the report are preserved, so an event without usage does
// not zero the accumulated tokens.
func applyReport(sess store.Session, isNew bool, req api.ReportRequest) store.Session {
	sess.ID = req.SessionID
	// Context fields are preserved when a later event omits them, so a partial
	// report (e.g. Stop without git_branch) does not erase known context.
	if req.Machine != "" {
		sess.Machine = req.Machine
	}
	if req.ProjectDir != "" {
		sess.ProjectDir = req.ProjectDir
	}
	if req.GitBranch != "" {
		sess.GitBranch = req.GitBranch
	}
	if req.Model != "" {
		sess.Model = req.Model
	}
	if req.Title != "" {
		sess.Title = req.Title
	}
	if isNew {
		sess.StartedAt = req.Timestamp
	}
	sess.LastSeenAt = req.Timestamp
	sess.Status = deriveStatus(req.Event, sess.Status)
	if req.Event == "SessionEnd" {
		sess.EndedAt = req.Timestamp
	}
	if req.LastTool != "" {
		sess.LastTool = req.LastTool
	}
	if req.Usage != nil {
		sess.Usage = store.Usage{
			InputTokens:         req.Usage.InputTokens,
			OutputTokens:        req.Usage.OutputTokens,
			CacheCreationTokens: req.Usage.CacheCreationTokens,
			CacheReadTokens:     req.Usage.CacheReadTokens,
		}
	}
	return sess
}

// deriveStatus maps a hook event to a session status, keeping the current
// status for events that do not change it.
func deriveStatus(event, current string) string {
	switch event {
	case "SessionStart":
		if current == "" {
			return "idle"
		}
		return current
	case "UserPromptSubmit":
		return "working"
	case "Notification":
		return "waiting_input"
	case "Stop":
		return "idle"
	case "SessionEnd":
		return "ended"
	default:
		// e.g. PostToolUse: activity, keep (or enter) the working state.
		if current == "" {
			return "working"
		}
		return current
	}
}
