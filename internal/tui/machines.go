package tui

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/haribo/claude-fleet/internal/api"
)

// machineStat aggregates one machine's sessions for the Machines tab.
type machineStat struct {
	name     string
	sessions int
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
			a = &machineStat{name: s.Machine}
			byName[s.Machine] = a
		}
		a.sessions++
		switch s.Status {
		case "working":
			a.working++
		case "waiting":
			a.waiting++
		case "idle":
			a.idle++
		case "ended":
			a.ended++
		}
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

// renderMachines renders the per-machine fleet overview (read-only).
func renderMachines(sessions []api.SessionView, width int) string {
	if len(sessions) == 0 {
		return dimStyle.Render("no sessions yet")
	}
	now := time.Now()

	var b strings.Builder
	b.WriteString("  " + headerStyle.Render(
		pad("MACHINE", 16)+padLeft("SESS", 6)+padLeft("WORK", 7)+padLeft("WAIT", 7)+
			padLeft("IDLE", 6)+padLeft("ENDED", 7)+padLeft("OUT", 10)+"   "+
			pad("USER", 12)+padLeft("SEEN", 6)) + "\n")
	b.WriteString(rule(width) + "\n")
	for _, a := range aggregateMachines(sessions) {
		b.WriteString("  " +
			pad(a.name, 16) +
			padLeft(strconv.Itoa(a.sessions), 6) +
			countCell(a.working, "working", 7) +
			countCell(a.waiting, "waiting", 7) +
			countCell(a.idle, "idle", 6) +
			countCell(a.ended, "ended", 7) +
			padLeft(humanizeTokens(a.out), 10) + "   " +
			userStyle.Render(pad(orDash(a.user), 12)) +
			dimStyle.Render(padLeft(relativeAge(a.lastSeen, now), 6)) + "\n")
	}
	return b.String()
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
