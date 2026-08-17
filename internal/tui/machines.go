package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/status"
)

// machineStat aggregates one machine's sessions for the Machines tab.
type machineStat struct {
	name     string
	sessions int
	// byStatus counts every status seen, keyed by its name — including one this
	// build has never heard of, since the server owns the vocabulary and a client
	// may be older than it. The four columns below read from it; anything else
	// non-zero is spelled out at the end of the row rather than folded into a
	// total that reads as zero (#509).
	byStatus map[string]int
	working  int
	waiting  int
	idle     int
	ended    int
	out      int64  // summed output tokens
	user     string // OS user (last non-empty seen)
	lastSeen string // most recent last-seen across the machine's sessions
}

// aggregateMachines groups sessions by machine, busiest (most sessions) first.
func aggregateMachines(sessions []api.SessionView) []machineStat {
	byName := map[string]*machineStat{}
	for _, s := range sessions {
		a := byName[s.Machine]
		if a == nil {
			a = &machineStat{name: s.Machine, byStatus: map[string]int{}}
			byName[s.Machine] = a
		}
		a.sessions++
		a.byStatus[s.Status]++
		a.out += s.Usage.OutputTokens
		if s.User != "" {
			a.user = s.User
		}
		if s.LastSeenAt > a.lastSeen {
			a.lastSeen = s.LastSeenAt
		}
	}
	out := make([]machineStat, 0, len(byName))
	for _, a := range byName {
		a.working, a.waiting = a.byStatus["working"], a.byStatus["waiting"]
		a.idle, a.ended = a.byStatus["idle"], a.byStatus["ended"]
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].sessions != out[j].sessions {
			return out[i].sessions > out[j].sessions
		}
		return out[i].name < out[j].name
	})
	return out
}

// versionCell renders a machine's watcher build, or a dash when none reported.
func versionCell(v api.VersionInfo) string {
	if v.Version == "" {
		return "—"
	}
	return v.Version
}

// renderMachines renders the per-machine fleet overview (read-only). watcherSeen
// maps each machine to the RFC3339 time of its last watch report, so machines
// running on hooks alone are flagged (#284); versions carries each watcher's
// build so a drifted watcher is visible (#356).
func renderMachines(sessions []api.SessionView, watcherSeen map[string]string, versions map[string]api.VersionInfo, width int) string {
	if len(sessions) == 0 {
		return dimStyle.Render("no sessions yet")
	}
	now := clock.Now() // presentation: relative "SEEN" ages

	var b strings.Builder
	// The overview table has no column-drop of its own; clamp each row to width so
	// it never scrolls sideways (#329).
	b.WriteString(clampWidth("  "+headerStyle.Render(
		pad("MACHINE", 16)+padLeft("SESS", 6)+padLeft("WORK", 7)+padLeft("WAIT", 7)+
			padLeft("IDLE", 6)+padLeft("ENDED", 7)+padLeft("OUT", 10)+"   "+
			pad("USER", 12)+padLeft("SEEN", 6)+"   "+pad("WATCH", 8)+"   "+pad("VERSION", 12)), width) + "\n")
	b.WriteString(rule(width) + "\n")
	var noWatcher []string
	for _, a := range aggregateMachines(sessions) {
		fresh := watcherFresh(watcherSeen[a.name], now)
		if !fresh {
			noWatcher = append(noWatcher, a.name)
		}
		b.WriteString(clampWidth("  "+
			pad(a.name, 16)+
			padLeft(strconv.Itoa(a.sessions), 6)+
			countCell(a.working, "working", 7)+
			countCell(a.waiting, "waiting", 7)+
			countCell(a.idle, "idle", 6)+
			countCell(a.ended, "ended", 7)+
			padLeft(humanizeTokens(a.out), 10)+"   "+
			userStyle.Render(pad(orDash(a.user), 12))+
			dimStyle.Render(padLeft(relativeAge(a.lastSeen, now), 6))+"   "+
			watchCell(fresh)+"   "+
			dimStyle.Render(pad(versionCell(versions[a.name]), 12))+
			otherStatuses(a.byStatus), width) + "\n")
	}
	// Only surface the banner when something is actually wrong — no watcher on at
	// least one machine — so a healthy fleet stays quiet. Prose wraps to width.
	if len(noWatcher) > 0 {
		warn := warnStyle.Render("⚠ no watcher on " + strings.Join(noWatcher, ", "))
		help := dimStyle.Render("  run `vigie watch` there (or enable its service) — statuses can go stale without it")
		if width > 0 {
			warn = lipgloss.NewStyle().Width(width).Render(warn)
			help = lipgloss.NewStyle().Width(width).Render(help)
		}
		b.WriteString("\n" + warn + "\n" + help + "\n")
	}
	return b.String()
}

// columnStatuses are the four that have a column of their own. Everything else a
// machine is running is spelled out by otherStatuses, so no session is counted in
// SESS and shown nowhere (#509).
var columnStatuses = map[string]bool{"working": true, "waiting": true, "idle": true, "ended": true}

// otherStatuses renders the statuses without a column, in the vocabulary's own
// order, and empty when there are none — so a fleet that fits the columns looks
// exactly as it did.
//
// A status the server knows and this build does not is included too: it sorts
// last (status.Rank), and showing an unfamiliar word beats dropping a session.
func otherStatuses(byStatus map[string]int) string {
	keys := make([]string, 0, len(byStatus))
	for st, n := range byStatus {
		if n > 0 && !columnStatuses[st] {
			keys = append(keys, st)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Slice(keys, func(i, j int) bool {
		if ri, rj := status.Rank(keys[i]), status.Rank(keys[j]); ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, st := range keys {
		parts = append(parts, statusStyle(st).Render("● "+st+" "+strconv.Itoa(byStatus[st])))
	}
	return "   " + strings.Join(parts, "  ")
}

// watcherFresh reports whether a machine's last watch report is recent enough to
// trust its statuses. Empty or unparseable → stale (#284).
func watcherFresh(seen string, now time.Time) bool {
	if seen == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, seen)
	if err != nil {
		return false
	}
	return now.Sub(t) <= watcherStaleAfter
}

// watchCell renders the per-machine watcher indicator: green "● live" when a
// fresh watcher reports, amber "⚠ none" otherwise.
func watchCell(fresh bool) string {
	if fresh {
		return watchLiveStyle.Render(pad("● live", 8))
	}
	return warnStyle.Render(pad("⚠ none", 8))
}

// countCell renders a status count right-aligned to w, dimmed when zero, else
// in the status color.
func countCell(n int, status string, w int) string {
	s := padLeft(strconv.Itoa(n), w)
	if n == 0 {
		return dimStyle.Render(s)
	}
	return statusStyle(status).Render(s)
}
