package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haribo/claude-vigie/internal/api"
	"github.com/haribo/claude-vigie/internal/modelinfo"
)

// statsView is the Stats tab's own state: the time granularity the buckets are
// rolled into (#379). The daily rows it renders are shared, fetched data on the
// model; only the chosen period is the tab's.
type statsView struct {
	period period
}

// handleKey folds a period-switch key (d w m y t) into the tab state, ignoring
// any other key.
func (v statsView) handleKey(msg tea.KeyMsg) statsView {
	if p, ok := periodFromKey(msg.String()); ok {
		v.period = p
	}
	return v
}

// period is the time granularity of the Stats view: daily rollups are bucketed
// client-side into day/week/month/year/total (switched with d w m y t).
type period int

const (
	periodDay period = iota
	periodWeek
	periodMonth
	periodYear
	periodTotal
	periodCount
)

var periodNames = [periodCount]string{"Day", "Week", "Month", "Year", "Total"}

// periodFromKey maps a keypress to a period; ok is false for other keys.
func periodFromKey(k string) (period, bool) {
	switch k {
	case "d":
		return periodDay, true
	case "w":
		return periodWeek, true
	case "m":
		return periodMonth, true
	case "y":
		return periodYear, true
	case "t":
		return periodTotal, true
	}
	return 0, false
}

// bucketCount is how many recent buckets a period shows.
func bucketCount(p period) int {
	switch p {
	case periodDay:
		return 14
	case periodWeek, periodMonth:
		return 12
	case periodYear:
		return 6
	default: // total
		return 1
	}
}

type statBucket struct {
	label   string
	tokens  map[string]int64 // by model
	total   int64
	working int64
	waiting int64
	idle    int64
}

// periodKey returns the bucket key (chronologically sortable) and a day used to
// derive the bucket's label.
func periodKey(day string, p period) (key, labelDay string) {
	switch p {
	case periodWeek:
		if t, err := time.Parse("2006-01-02", day); err == nil {
			y, w := t.ISOWeek()
			return fmt.Sprintf("%04d-W%02d", y, w), day
		}
	case periodMonth:
		if len(day) >= 7 {
			return day[:7], day
		}
	case periodYear:
		if len(day) >= 4 {
			return day[:4], day
		}
	case periodTotal:
		return "all", day
	}
	return day, day
}

func bucketLabel(labelDay string, p period) string {
	if p == periodTotal {
		return "all"
	}
	t, err := time.Parse("2006-01-02", labelDay)
	if err != nil {
		return labelDay
	}
	switch p {
	case periodWeek:
		_, w := t.ISOWeek()
		return fmt.Sprintf("W%02d", w)
	case periodMonth:
		return t.Format("Jan")
	case periodYear:
		return t.Format("2006")
	default:
		return t.Format("Jan 02")
	}
}

// bucketStats groups daily rows into the period's buckets and returns the most
// recent bucketCount(p), oldest first.
func bucketStats(daily []api.DailyStat, p period) []statBucket {
	byKey := map[string]*statBucket{}
	var keys []string
	for _, d := range daily {
		key, labelDay := periodKey(d.Day, p)
		b := byKey[key]
		if b == nil {
			b = &statBucket{label: bucketLabel(labelDay, p), tokens: map[string]int64{}}
			byKey[key] = b
			keys = append(keys, key)
		}
		b.tokens[d.Model] += d.OutputTokens
		b.total += d.OutputTokens
		b.working += d.WorkingSeconds
		b.waiting += d.WaitingSeconds
		b.idle += d.IdleSeconds
	}
	sort.Strings(keys)
	out := make([]statBucket, 0, len(keys))
	for _, k := range keys {
		out = append(out, *byKey[k])
	}
	if n := bucketCount(p); len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// statModels returns the models the chart actually draws, sorted: those carrying
// at least one token in the buckets on screen.
//
// It reads the rendered buckets, not the whole history, and that is the fix for
// #845. Read from history it named every model ever seen — including, under the
// `day` period, a bucket whose only rows were five weeks old — while the bars were
// truncated to the period. Six legend entries, four models drawn.
//
// The token test is the second half. An empty-model bucket is real in the store —
// it means "not yet known" and carries status time (docs/design/token-rollup.md)
// — but it can never carry a token, so in a chart of tokens it is a legend entry
// for a quantity it cannot have. `renderStackBar` already skips a zero; this
// applies the same test one place earlier.
func statModels(buckets []statBucket) []string {
	seen := map[string]bool{}
	var out []string
	for _, b := range buckets {
		for mdl, v := range b.tokens {
			if v == 0 || seen[mdl] {
				continue
			}
			seen[mdl] = true
			out = append(out, mdl)
		}
	}
	sort.Strings(out)
	return out
}

// colorForModel is a model's color, fixed by its position among every model in the
// history rather than among the ones currently drawn.
//
// Otherwise a model changes color when the period changes, because filtering the
// legend (above) shifts every index after the one removed. The palette is what an
// operator carries from one screen to the next, so it is keyed on something the
// period does not move.
func colorForModel(daily []api.DailyStat, mdl string) lipgloss.AdaptiveColor {
	seen := map[string]bool{}
	var all []string
	for _, d := range daily {
		if !seen[d.Model] {
			seen[d.Model] = true
			all = append(all, d.Model)
		}
	}
	sort.Strings(all)
	for i, m := range all {
		if m == mdl {
			return modelColor(i)
		}
	}
	return modelColor(0)
}

var statPalette = []lipgloss.AdaptiveColor{cAccent2, cAccent, cGreen, cAmber, cRed}

func modelColor(i int) lipgloss.AdaptiveColor { return statPalette[i%len(statPalette)] }

// fmtHM formats a duration in seconds as "Xh YYm" (or "Ym" under an hour).
func fmtHM(secs int64) string {
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func clip(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

func (m model) renderStats() string {
	var b strings.Builder
	b.WriteString(m.staleNote(srcStats))

	// Period switcher.
	parts := make([]string, periodCount)
	for p := period(0); p < periodCount; p++ {
		lbl := " " + periodNames[p] + " "
		if p == m.stat.period {
			parts[p] = cursorStyle.Render(lbl)
		} else {
			parts[p] = dimStyle.Render(lbl)
		}
	}
	b.WriteString(labelStyle.Render("Period ") + strings.Join(parts, "") + "\n\n")

	daily := m.stats.Daily
	if len(daily) == 0 {
		// "stats accumulate as sessions report" was true of a fleet that had just
		// started and false of one covered only by the watcher, where the durations
		// never accumulate at all — only hooks close a status interval
		// (status-time.md § 2). The two want different things from the operator:
		// waiting, versus installing the hooks (#668).
		b.WriteString(dimStyle.Render("No activity yet — tokens accrue from any report;") + "\n")
		b.WriteString(dimStyle.Render("time needs the reporting hooks (`vigie hooks install`)."))
		return b.String()
	}

	buckets := bucketStats(daily, m.stat.period)
	models := statModels(buckets)

	var totTok, totWork, totWait, totIdle int64
	waitSeries := make([]int, 0, len(buckets))
	maxTot := int64(1)
	for _, bk := range buckets {
		totTok += bk.total
		totWork += bk.working
		totWait += bk.waiting
		totIdle += bk.idle
		waitSeries = append(waitSeries, int(bk.waiting))
		if bk.total > maxTot {
			maxTot = bk.total
		}
	}

	// Hero + KPIs.
	b.WriteString(dimStyle.Render(pad("BOTTLENECK", 18)+pad("TOKENS", 12)+pad("ACTIVE", 12)+"SESSIONS") + "\n")
	b.WriteString(statusStyle("waiting").Bold(true).Render(pad(fmtHM(totWait), 18)) +
		pad(humanizeTokens(totTok), 12) + pad(fmtHM(totWork), 12) +
		fmt.Sprintf("%d", m.stats.SessionCount) + "\n")
	b.WriteString(statusStyle("waiting").Render(sparkline(waitSeries)) +
		dimStyle.Render("  waiting per bucket") + "\n\n")

	// Tokens per bucket, stacked by model.
	b.WriteString(dimStyle.Render("TOKENS  ·  by model") + "\n")
	legend := make([]string, len(models))
	for i, mdl := range models {
		name := modelinfo.Short(mdl)
		if name == "" {
			// Unreachable while the store's guarantee holds — an unknown model implies
			// no tokens, and a model with no tokens is not in `models`. Kept so that if
			// it ever stops holding, the row shows rather than a blank swatch.
			name = "—"
		}
		legend[i] = lipgloss.NewStyle().Foreground(colorForModel(daily, mdl)).Render("■ ") + dimStyle.Render(name)
	}
	b.WriteString(strings.Join(legend, "  ") + "\n")
	for _, bk := range buckets {
		b.WriteString(dimStyle.Render(pad(bk.label, 8)) + renderStackBar(bk, models, daily, maxTot) +
			"  " + humanizeTokens(bk.total) + "\n")
	}
	b.WriteString("\n")

	// Where time went.
	b.WriteString(dimStyle.Render("WHERE TIME WENT") + "\n")
	b.WriteString(renderStatusBar(totWork, totWait, totIdle) + "\n")
	b.WriteString(statusStyle("idle").Render("idle "+fmtHM(totIdle)) + dimStyle.Render(" · ") +
		statusStyle("working").Render("active "+fmtHM(totWork)) + dimStyle.Render(" · ") +
		statusStyle("waiting").Render("waiting "+fmtHM(totWait)) + "\n\n")

	b.WriteString(renderTopSessions(m.stats.TopSessions))
	return b.String()
}

const statBarWidth = 26

func renderStackBar(bk statBucket, models []string, daily []api.DailyStat, maxTot int64) string {
	barLen := int(bk.total * int64(statBarWidth) / maxTot)
	if bk.total > 0 && barLen == 0 {
		barLen = 1
	}
	var sb strings.Builder
	used := 0
	for _, mdl := range models {
		v := bk.tokens[mdl]
		if v == 0 {
			continue
		}
		n := int(v * int64(statBarWidth) / maxTot)
		if n == 0 {
			n = 1
		}
		if used+n > barLen {
			n = barLen - used
		}
		if n <= 0 {
			continue
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(colorForModel(daily, mdl)).Render(strings.Repeat("█", n)))
		used += n
	}
	if used < statBarWidth {
		sb.WriteString(dimStyle.Render(strings.Repeat("░", statBarWidth-used)))
	}
	return sb.String()
}

func renderStatusBar(work, wait, idle int64) string {
	const w = 40
	tot := work + wait + idle
	if tot == 0 {
		return dimStyle.Render(strings.Repeat("░", w))
	}
	iw := int(idle * int64(w) / tot)
	ww := int(work * int64(w) / tot)
	wa := w - iw - ww
	if wa < 0 {
		wa = 0
	}
	return statusStyle("idle").Render(strings.Repeat("█", iw)) +
		statusStyle("working").Render(strings.Repeat("█", ww)) +
		statusStyle("waiting").Render(strings.Repeat("█", wa))
}

func renderTopSessions(top []api.TopSession) string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("TOP SESSIONS") + "\n")
	b.WriteString(dimStyle.Render(pad("SESSION", 22)+pad("MACHINE", 14)+pad("MODEL", 12)+pad("STATUS", 11)+"TOKENS") + "\n")
	if len(top) == 0 {
		b.WriteString(dimStyle.Render("  none"))
		return b.String()
	}
	for _, s := range top {
		b.WriteString(pad(clip(s.Name, 21), 22) +
			dimStyle.Render(pad(orDash(s.Machine), 14)) +
			dimStyle.Render(pad(modelinfo.Short(s.Model), 12)) +
			statusStyle(s.Status).Render(pad("● "+s.Status, 11)) +
			humanizeTokens(s.OutputTokens) + "\n")
	}
	return b.String()
}

func (m model) handleStatsKey(msg tea.KeyMsg) model {
	m.stat = m.stat.handleKey(msg)
	return m
}
