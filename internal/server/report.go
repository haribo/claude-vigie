package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/status"
	"github.com/haribo/claude-vigie/internal/store"
)

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	var req api.ReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			metricReportsRejected.WithLabelValues("too_large").Inc()
			s.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		metricReportsRejected.WithLabelValues("bad_json").Inc()
		s.writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.SessionID == "" || req.Event == "" {
		metricReportsRejected.WithLabelValues("missing_fields").Inc()
		s.writeError(w, http.StatusBadRequest, "session_id and event are required")
		return
	}

	ctx := r.Context()

	// A drifted watcher may not write session state (#384). Its build travels in
	// every watch report (#356), so the daemon — the authority on its own build —
	// arbitrates here, where an outdated client cannot bypass the check. The
	// machine's presence and its faulty build are still recorded, so the fleet can
	// see which machine to upgrade; only the session data is refused. Hook reports
	// declare no build and stay ungated on purpose: they run inside the operator's
	// Claude session (docs/design/version-consistency.md).
	if req.Event == "watch" && !watcherBuildMatches(req) {
		s.recordWatchHeartbeat(ctx, req)
		metricReportsRejected.WithLabelValues("version_drift").Inc()
		s.writeError(w, http.StatusConflict, driftMessage(req))
		return
	}

	if reason, msg := rejectReport(req); reason != "" {
		metricReportsRejected.WithLabelValues(reason).Inc()
		s.writeError(w, http.StatusBadRequest, msg)
		return
	}

	// Read, merge and write as one step: nothing may land between them, or the
	// merge decides from a state another report has already replaced (#512).
	var existing store.Session
	var isNew bool
	sess, err := s.store.ApplySession(ctx, req.SessionID, func(current store.Session, fresh bool) store.Session {
		existing, isNew = current, fresh
		merged := applyReport(current, fresh, req)
		merged.ReportedAt = s.now().UTC().Format(time.RFC3339) // server-side heartbeat
		return merged
	})
	if err != nil {
		s.log.Error("applying report", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Tokens accrue on every report (analytics approach A): attribute the
	// output-token delta to today's daily rollup.
	s.rollupTokens(ctx, existing, sess, req)

	// The watcher polls frequently; keep its scans out of the event log, but
	// record a heartbeat so clients can detect an absent watcher.
	if req.Event == "watch" {
		s.recordWatchHeartbeat(ctx, req)
	} else {
		// A hook event closes the previous status interval; roll up its
		// duration before recording the transition.
		s.rollupStatusInterval(ctx, sess, req)
		if err := s.store.AppendEvent(ctx, store.Event{
			SessionID: sess.ID, Event: req.Event, Status: sess.Status, CreatedAt: req.Timestamp,
		}); err != nil {
			// The session is already updated; the event log is best-effort.
			s.log.Error("appending event", "error", err)
		}
	}

	s.maybeSample(ctx, sess.ID, req.Timestamp, sess.Usage.OutputTokens)
	// Delta-gate the SSE fan-out (#258): the watcher re-reports every session
	// every ~2 s, mostly changing nothing but the heartbeat. Publish only when the
	// operator-visible state actually changed (or the session is new), so idle
	// clients stop refetching sessions/usage/stats/platform on every no-op scan.
	// The heartbeat still lives in ReportedAt; the stale→ended cutoff is evaluated
	// at read time, and the TUI's own 5 s tick refreshes relative ages.
	if isNew || visibleSignature(existing) != visibleSignature(sess) {
		s.hub.publish()
	}

	metricReports.WithLabelValues(req.Event).Inc()
	w.WriteHeader(http.StatusNoContent)
}

// rollupTokens adds the session's output-token growth since the last report to
// today's daily rollup, bucketed by model. Best-effort.
func (s *Server) rollupTokens(ctx context.Context, _, sess store.Session, req api.ReportRequest) {
	// Count against a mark of our own rather than the growth of the session row.
	// That row is a counter the rollup does not own: it regresses when one session
	// is written to two transcript files, when a transcript is truncated, or when
	// retention deletes the row while the transcript lives on — and each
	// regression made the next report look like a whole lifetime of fresh output,
	// permanently, since stats_daily is never recomputed (#432,
	// docs/design/token-rollup.md).
	//
	// The mark and the daily write commit together (#669). Separately, a mark that
	// advanced over a failed write said the growth had been counted when it had
	// not — permanently, since stats_daily is never recomputed. Together, a failure
	// leaves the mark where it was and the next report counts the same growth.
	delta, err := s.store.RollUpTokens(ctx, sess.ID, sess.Usage.OutputTokens,
		dayOf(req.Timestamp, s.now()), sess.Model)
	if err != nil {
		s.log.Error("rolling up tokens", "error", err, "session", sess.ID)
		return
	}
	if delta <= 0 {
		return
	}
	// After the commit, not before: the metric counts what landed. It used to be
	// incremented between the two writes, so a failed daily write left the metric
	// ahead of the table it reports on.
	metricOutputTokens.WithLabelValues(modelLabel(sess.Model)).Add(float64(delta)) // bounded: see modelFamilies (#528)
}

// rollupStatusInterval closes the interval since the session's previous event by
// adding its duration to that status's daily bucket. Only hook events carry
// status transitions, so the watcher's polls never reach here. Best-effort.
func (s *Server) rollupStatusInterval(ctx context.Context, sess store.Session, req api.ReportRequest) {
	last, ok, err := s.store.LastEvent(ctx, sess.ID)
	if err != nil || !ok {
		return
	}
	secs := secondsBetween(last.CreatedAt, req.Timestamp)
	if secs <= 0 {
		return
	}
	// Attribute the whole interval to its start day (no midnight split in v1).
	if err := s.store.AddDailyStatusSeconds(ctx, dayOf(last.CreatedAt, s.now()), sess.Model, last.Status, secs); err != nil {
		s.log.Error("rolling up daily status", "error", err)
	}
}

// dayOf returns the UTC calendar day (YYYY-MM-DD) of an RFC3339 timestamp,
// falling back to the current day when it cannot be parsed.
func dayOf(ts string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t = now
	}
	return t.UTC().Format("2006-01-02")
}

// secondsBetween returns the whole seconds from RFC3339 timestamp from to to,
// or 0 if either is unparseable or the span is non-positive.
func secondsBetween(from, to string) int64 {
	a, err1 := time.Parse(time.RFC3339, from)
	b, err2 := time.Parse(time.RFC3339, to)
	if err1 != nil || err2 != nil {
		return 0
	}
	if d := b.Sub(a); d > 0 {
		return int64(d.Seconds())
	}
	return 0
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

// visibleSignature is a fingerprint of everything the dashboard shows for a
// session *except* the pure heartbeats (ReportedAt, LastSeenAt), which change on
// every report and are refreshed by the client's own tick. Two sessions with the
// same signature are indistinguishable on screen, so an SSE push between them is
// wasted (#258).
func visibleSignature(s store.Session) string {
	u := s.Usage
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s|%t|%s|%d|%s|%s|%d|%d|%d|%d|%s|%d|%t|%s",
		s.Status, s.StatusChangedAt, s.Detail, s.Title, s.User, s.Machine, s.Model,
		s.GitBranch, s.ProjectDir, s.LastTool, s.RemoteControl, s.RemoteURL, s.APIErrorStatus,
		s.CallAt, s.CallMessage, // raising or clearing a call must reach the dashboards (#388)
		u.InputTokens, u.OutputTokens, u.CacheCreationTokens, u.CacheReadTokens,
		// The dashboard renders these too, and stops polling once the stream is
		// live — so a session switching to plan mode, changing effort or filling
		// its context never redrew while they were missing (#514). ContextKnown
		// travels separately from the figure: "unknown" and "known to be zero"
		// are different states on screen (#367).
		s.Effort, s.ContextTokens, s.ContextKnown, s.PermissionMode)
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
	if req.Effort != "" {
		sess.Effort = req.Effort
	}
	if req.ContextTokens != nil { // a known reading (incl. 0) overwrites; nil keeps the last known (#367)
		sess.ContextTokens = *req.ContextTokens
		sess.ContextKnown = true
	}
	if req.PermissionMode != "" { // keep the last known mode when a report carries none
		sess.PermissionMode = req.PermissionMode
	}
	if req.Title != "" {
		sess.Title = req.Title
	}
	if isNew {
		sess.StartedAt = req.Timestamp
	}
	sess.LastSeenAt = req.Timestamp

	sess = applyCall(sess, req)

	sess = applyStatus(sess, req)
	sess = applyEndedAt(sess, req)
	if req.RemoteControl != nil {
		sess.RemoteControl = *req.RemoteControl       // detected /rc state (read-only)
		sess.RemoteURL = safeRemoteURL(req.RemoteURL) // resume URL travels with the /rc flag; "" clears it (#515)
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

// applyCall folds a call into the session (ADR-0010). The session raises it, and
// the session clears it — by resuming work or by ending. No action on vigie is
// ever involved, which is what keeps the call on the right side of ADR-0007.
//
// Clearing keys on these events rather than on the session merely being
// `working`: Claude is still inside an active turn when it raises the call, so a
// status-based rule would erase the call the instant it was made.
func applyCall(sess store.Session, req api.ReportRequest) store.Session {
	switch req.Event {
	case "call":
		sess.CallMessage, sess.CallAt = req.CallMessage, req.Timestamp
	case "UserPromptSubmit", "SessionEnd":
		sess.CallMessage, sess.CallAt = "", ""
	}
	return sess
}

// reportDetail is the report's contextual detail, accepting the pre-#393 field
// name from a reporter that predates the rename (the hook reporter is
// deliberately ungated by the version check, so it can lag the daemon).
func reportDetail(req api.ReportRequest) string {
	if req.Detail != "" {
		return req.Detail
	}
	return req.Activity
}

// idleDetails are the DETAIL values a *resting* session legitimately carries.
//
// The #236 rule blanks the activity on idle, because a turn's "doing" message
// means nothing once the turn is over. These two describe the rest itself rather
// than a turn, so the rule does not apply to them:
//
//   - `shell` — the operator dropped to a shell inside Claude. The session is
//     idle but not free (#280);
//   - `interrupted` — the last turn was killed rather than finished, which is
//     the whole distinction the marker exists to draw. The watcher had been
//     sending it since #351 and this blanked it on arrival, so it never reached
//     a screen while a design document and a changelog entry described it (#659).
//
// Both clear the same way, with no timer: the next report that does not carry
// one leaves the session with an empty DETAIL.
var idleDetails = map[string]bool{"shell": true, "interrupted": true}

// applyEndedAt records when a session ended, and un-records it when one comes
// back.
//
// The end time used to be written once and never cleared, so it outlived the end
// itself: it is served to every client (`EndedAt` in the session view), and a
// resumed session carried the timestamp of an end it had already left behind
// (#664). A `SessionStart` on a live session finds it empty already, so the clear
// costs nothing where there is nothing to clear.
func applyEndedAt(sess store.Session, req api.ReportRequest) store.Session {
	switch req.Event {
	case "SessionEnd":
		sess.EndedAt = req.Timestamp
	case "SessionStart":
		sess.EndedAt = ""
	}
	return sess
}

// applyStatus folds the report's status into the session: the reconciled status
// and its owner, when it last changed, and the transient activity message.
func applyStatus(sess store.Session, req api.ReportRequest) store.Session {
	// A call carries no status and is orthogonal to it (ADR-0010), so it must
	// neither derive one nor take ownership of it: deriving would invent "working"
	// for a session that has none yet, and stamping the source "hook" would let a
	// watcher-set state latch — the watcher could no longer retract its own (the
	// #201 failure mode).
	if req.Event == "call" {
		return sess
	}
	prev := sess.Status
	if req.Status != "" {
		if !holdsWaiting(sess, req) {
			sess.Status, sess.StatusSource = reconcileWatch(sess.Status, sess.StatusSource, req.Status)
		}
		sess.APIErrorStatus = req.APIErrorStatus // watcher-derived; hooks carry no status
	} else {
		sess.Status = deriveStatus(req.Event, req.NotificationType, sess.Status)
		sess.StatusSource = "hook" // a hook event is the authoritative observer
	}
	if sess.Status != prev {
		sess.StatusChangedAt = req.Timestamp
	}
	// Activity describes an active turn; a resting status carries none (#236). So
	// clear it on idle/ended, take a fresh message when the report has one, else
	// drop a stale one on any status change so a new episode never shows the old
	// "doing".
	detail := reportDetail(req)
	switch {
	case sess.Status == "idle" && idleDetails[detail]:
		sess.Detail = detail
	case sess.Status == "idle" || sess.Status == "ended":
		sess.Detail = "" // clears once the report stops carrying it
	case detail != "":
		sess.Detail = detail
	case sess.Status != prev:
		sess.Detail = ""
	}
	return sess
}

// watcherObserves are the statuses the watcher establishes positively rather
// than infers from a quiet transcript: the transcript carries an API error, or
// the process is gone. They clear a hook-set `waiting` even while the transcript
// is frozen (session-status.md § 3).
//
// Everything else the watcher can report about a frozen transcript is inferred
// from silence — and a frozen transcript is what a permission prompt looks like.
// The set is a *deny* list on purpose: it was an allow list of
// working/thinking/compacting, so `stalled` (#256) fell straight through it when
// it was added, and a permission prompt read as a hung tool for the rest of the
// session (#508). A status added tomorrow is held by default, which is the safe
// direction — the cost of holding one too long is a late release, the cost of
// letting one through is telling the operator the wrong thing.
var watcherObserves = map[string]bool{"error": true, "ended": true}

// holdsWaiting reports whether a hook-set `waiting` must survive a watcher
// report (#235). The watcher can't tell "a tool is running" from "a permission
// prompt is blocking": both are a turn stopped on tool_use with a frozen
// transcript. So an inferred status may only clear waiting once the transcript
// has actually moved past when waiting was posted — i.e. the report's timestamp
// (the transcript mtime) is newer than StatusChangedAt.
func holdsWaiting(sess store.Session, req api.ReportRequest) bool {
	if sess.Status != "waiting" || sess.StatusSource != "hook" || sess.StatusChangedAt == "" {
		return false
	}
	if watcherObserves[req.Status] {
		return false
	}
	return !timeAfter(req.Timestamp, sess.StatusChangedAt)
}

// timeAfter reports whether RFC3339 time a is strictly after b, and false when
// either will not parse.
//
// Read that at the one place it is called: holdsWaiting *negates* it, so an
// absent or unreadable timestamp holds the waiting rather than releasing it.
// That is the right way round — with no timestamp there is no evidence the
// transcript moved, and this rule exists to demand that evidence — but the
// comment here used to claim the opposite, which is the kind of sentence someone
// corrects the code to match (#636).
func timeAfter(a, b string) bool {
	ta, err1 := time.Parse(time.RFC3339, a)
	tb, err2 := time.Parse(time.RFC3339, b)
	if err1 != nil || err2 != nil {
		return false
	}
	return ta.After(tb)
}

// reconcileWatch folds a watcher-reported status into the session, resolving the
// overlap between the two observers by *authority* rather than a status table.
//
// The watcher polls the transcript and process; it is authoritative for coverage
// and for anything it can positively observe (working, thinking, error, ended).
// But it only ever sees a quiet-but-alive session as "idle", and it cannot see
// two things a hook can: that the operator is the blocker (waiting), or that a
// turn is open while Claude works silently. So the watcher's "idle" must not
// retract a *hook-owned* active state — yet it must retract its *own* stale
// state, which is the finished-session latch bug (#201): a watcher-set "working"
// has to fall back to idle when the transcript goes quiet.
//
// It returns the new status and its owning source.
func reconcileWatch(current, currentSource, incoming string) (status, source string) {
	if incoming == "idle" && currentSource == "hook" &&
		(current == "waiting" || current == "working" || current == "thinking") {
		return current, "hook" // keep the hook's semantic/active state
	}
	if incoming == current {
		return current, currentSource // a confirmation doesn't transfer ownership
	}
	return incoming, "watch" // a positive change is the watcher's
}

// rejectReport validates a report's own fields, returning a metric reason and an
// operator-facing message when it must be refused, or "" when it may proceed.
//
// It runs after the version gate on purpose: a drifted watcher is a *who*
// question with a deliberate side effect (its heartbeat is still recorded so the
// fleet can see which machine to upgrade), while this is a *what* question about
// the payload.
func rejectReport(req api.ReportRequest) (reason, message string) {
	// An unknown event used to fall through deriveStatus's default arm to
	// `working`, *and* be stamped hook-owned — which the watcher could then no
	// longer retract, the #201 failure mode. Both fields are checked against the
	// vocabularies that exist, rather than trusted because the caller holds the
	// token (#515).
	switch {
	case !knownEvents[req.Event]:
		return "unknown_event", "unknown event"
	case req.Status != "" && !status.Known(req.Status):
		return "unknown_status", "unknown status"
	// The server tells its two informants apart by whether a report carries a
	// status: one that does not is taken for a hook and believed on its word — its
	// state is stamped `hook`, which the watcher may then never retract.
	//
	// A watch report's whole contribution *is* the status it inferred, so an empty
	// one says nothing at all; believed anyway, it invents `working` on a new
	// session and locks it there for the rest of its life (#201, #527). The real
	// watcher always carries one — statusFor cannot return an empty string — so
	// only a malformed report is refused here.
	case req.Event == "watch" && req.Status == "":
		return "watch_without_status", "a watch report must carry a status"
	// The report carries one timestamp and the server copies it into five fields of
	// the session view, three of which the TUI's detail panel prints as they came.
	// An OSC sequence there set the title of the operator's terminal window — #540
	// again, in the fields that do not look like text (#629).
	//
	// Refusing it here rather than cleaning it at each screen also keeps a value
	// that is not an instant out of three places that key on it and do not check
	// it, none of which merely ignore it:
	//
	//   - the events table stores it verbatim — plain INSERT, no constraint;
	//   - `token_samples` is keyed on it and pruned by `ORDER BY at DESC LIMIT`,
	//     so a string that sorts high evicts real samples;
	//   - `dayOf` falls back to *now* rather than failing, so the tokens land on
	//     today in `stats_daily`, which is never recomputed (#432).
	//
	// Empty stays accepted, on the model of the status check above: absent is not
	// malformed, it renders as a dash, and it cannot act on a terminal.
	case req.Timestamp != "" && !isRFC3339(req.Timestamp):
		return "invalid_timestamp", "timestamp must be an RFC3339 instant"
	}
	return "", ""
}

// isRFC3339 reports whether s is an instant vigie can reason about. Strict on
// purpose: a bare date or a time with no zone parses in JavaScript and not in Go,
// so accepting one would put a value on screen that the two clients read
// differently (ADR-0011's whole subject).
func isRFC3339(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

// knownEvents are the events vigie emits: the Claude Code hooks it installs
// (internal/client.defaultEvents), plus the watcher's scan and a session-raised
// call. Anything else is a malformed or hostile report, not a future feature — a
// new hook is added here at the same time as it is installed (#515).
var knownEvents = map[string]bool{
	"SessionStart": true, "UserPromptSubmit": true, "PostToolUse": true,
	"Notification": true, "Stop": true, "PreCompact": true, "SessionEnd": true,
	"watch": true, "call": true,
}

// safeRemoteURL returns the /rc resume URL if it is one a browser may safely be
// pointed at, else "". It is validated here rather than at render: the dashboard
// puts it in an href, and `javascript:` or `data:` survives HTML escaping
// untouched — escaping stops an attribute being broken out of, not a scheme from
// being followed. Validating at ingestion means a bad value never reaches the
// store, so no client has to remember to check (#515).
//
// Only https is allowed. The URL is Claude's own resume link (ADR-0005: detected,
// never set), so anything else is not a URL vigie has any business relaying.
func safeRemoteURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return raw
}

// deriveStatus maps a hook event to a session status, keeping the current
// status for events that do not change it. notifType is the Notification hook's
// notification_type; it splits "waiting on a human" from "idle".
func deriveStatus(event, notifType, current string) string {
	switch event {
	case "SessionStart":
		// The event says a session exists, not what it is doing, so it keeps
		// whatever status the session already has — with one exception. `ended` is
		// the value a SessionStart is direct evidence against: the session is
		// starting, so it is not over, and a resume (`claude --resume` keeps the
		// id) used to come back reading `ended` until the operator typed
		// something (#664).
		if current == "" || current == "ended" {
			return "idle"
		}
		return current
	case "UserPromptSubmit":
		return "working"
	case "Notification":
		// A Notification means Claude wants attention. Only idle_prompt (finished,
		// awaiting the next prompt) is truly idle; permission_prompt and the rest
		// mean the operator is the blocker. Empty (older Claude Code) stays waiting.
		if notifType == "idle_prompt" {
			return "idle"
		}
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
