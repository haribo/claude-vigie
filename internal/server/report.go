package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

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
	sess.ReportedAt = time.Now().UTC().Format(time.RFC3339) // server-side heartbeat
	if err := s.store.UpsertSession(ctx, sess); err != nil {
		s.log.Error("upserting session", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// The watcher polls frequently; keep its scans out of the event log, but
	// record a heartbeat so clients can detect an absent watcher.
	if req.Event == "watch" {
		if err := s.store.SetMeta(ctx, watchSeenKey, time.Now().UTC().Format(time.RFC3339)); err != nil {
			s.log.Error("recording watch heartbeat", "error", err)
		}
	} else if err := s.store.AppendEvent(ctx, store.Event{
		SessionID: sess.ID, Event: req.Event, Status: sess.Status, CreatedAt: req.Timestamp,
	}); err != nil {
		// The session is already updated; the event log is best-effort.
		s.log.Error("appending event", "error", err)
	}

	s.maybeSample(ctx, sess.ID, req.Timestamp, sess.Usage.OutputTokens)
	s.hub.publish()

	w.WriteHeader(http.StatusNoContent)
}

// maybeSample records a token sample at most once per minute per session, so
// the watcher's frequent reports don't flood the samples table.
func (s *Server) maybeSample(ctx context.Context, sessionID, at string, output int64) {
	if last, err := s.store.LastSampleAt(ctx, sessionID); err == nil && last != "" {
		lt, e1 := time.Parse(time.RFC3339, last)
		nt, e2 := time.Parse(time.RFC3339, at)
		if e1 == nil && e2 == nil && nt.Sub(lt) < time.Minute {
			return
		}
	}
	if err := s.store.AddSample(ctx, sessionID, at, output); err != nil {
		s.log.Error("adding sample", "error", err)
	}
}

// applyReport merges a report into the session state (read-modify-write).
// Fields absent from the report are preserved, so an event without usage does
// not zero the accumulated tokens.
func applyReport(sess store.Session, isNew bool, req api.ReportRequest) store.Session {
	sess.ID = req.SessionID
	// Context fields are preserved when a later event omits them, so a partial
	// report (e.g. Stop without git_branch) does not erase known context.
	if req.User != "" {
		sess.User = req.User
	}
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
	if req.Status != "" {
		sess.Status = mergeStatus(sess.Status, req.Status)
	} else {
		sess.Status = deriveStatus(req.Event, sess.Status)
	}
	if req.Event == "SessionEnd" {
		sess.EndedAt = req.Timestamp
	}
	if req.RemoteControl != nil {
		sess.RemoteControl = *req.RemoteControl // detected /rc state (read-only)
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

// mergeStatus applies an explicit (watcher) status, but keeps a "waiting"
// session waiting when the watcher only sees it as "idle" (alive but between
// turns). "waiting" is a semantic state — Claude asked for input — that the
// watcher cannot detect, so it persists until real activity resumes or the
// session ends.
func mergeStatus(current, incoming string) string {
	if current == "waiting" && incoming == "idle" {
		return "waiting"
	}
	return incoming
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
		return "waiting"
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
