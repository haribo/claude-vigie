// Pure helpers for the vigie indicator: no GNOME imports, no state, no I/O.
// extension.js keeps everything that touches Shell; this file is what a test can
// exercise under node (#430).

// groupOrder takes the known order as an argument rather than reading a
// module-level constant: the list itself stays in extension.js, where a Go test
// checks it against the design document (#423), and this stays a pure function a
// node test can state its own inputs for (#430).
//
// groupOrder returns the statuses to render: the known ones in their documented
// order, then anything else the server actually sent. Keeping the list in step is
// what the Go test enforces, but a menu must not depend on being in step — a
// status added on the server side has to *appear*, unstyled and unlabelled if
// need be, rather than take its sessions off the screen (#422).
export function groupOrder(sessions, statusOrder) {
    const seen = new Set(sessions.map(s => s.status).filter(Boolean));
    const unknown = [...seen].filter(s => !statusOrder.includes(s)).sort();
    return [...statusOrder, ...unknown];
}

export function basename(path) {
    if (!path)
        return '';
    const parts = path.replace(/\/+$/, '').split('/');
    return parts[parts.length - 1] || path;
}

// The statuses that call for the operator: the session is blocked and needs a
// human. Kept identical to internal/status.Attention — a Go test reads this
// literal and fails on drift, because an indicator that disagrees with the TUI
// about when to interrupt you is worse than no indicator (#466).
export const ATTENTION = ["waiting", "error", "stalled"];

// needsAttention covers both reasons to interrupt: a status that means the
// session is blocked, and a call the session raised for itself (ADR-0010). The
// call is not a status — it rides alongside one — so anything deciding whether to
// interrupt has to look at both.
export function needsAttention(session) {
  if (!session) return false;
  return Boolean(session.call_at) || ATTENTION.includes(session.status);
}

// attentionReason says why, for the notification body. A raised call outranks the
// status: it is the session speaking, not an inference about it.
export function attentionReason(session) {
  if (!session) return "";
  if (session.call_at) return session.call_message || "called you";
  switch (session.status) {
    case "waiting": return "is waiting for input";
    case "stalled": return "is stalled on a tool";
    case "error":   return "hit an API error";
    default:        return "";
  }
}

// attentionIds is the set of sessions currently calling for the operator, used to
// notify on the transition into that set rather than on every poll.
export function attentionIds(sessions) {
  return new Set((sessions || []).filter(needsAttention).map((s) => s.id));
}
