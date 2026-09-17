package usage

import "testing"

// #840. Claude enforces a third limit the strip never showed: a weekly one scoped
// to a single model. Observed live at 98 % while `5h` sat at 21 % and `7d` at 73 %
// — the board read comfortable while the limit that would actually stop the work
// was nearly full.
//
// **The scope is read from the entry, never from a field named after a model.**
// The payload still carries flat `seven_day_opus` / `seven_day_sonnet` fields and
// both are null: that shape was abandoned upstream, and reviving it here would be
// the hand-kept list #821 is about — the next model to carry a limit would be
// invisible in turn, exactly as a persistent `Monitor` was invisible to
// `isBackground` before #834.
//
// Line shapes below are the observed ones, with this account's own figures and
// nothing else from the payload.

const payloadScoped = `{
  "five_hour": {"utilization": 21.0, "resets_at": "2026-09-17T17:30:00Z"},
  "seven_day": {"utilization": 73.0, "resets_at": "2026-09-19T03:00:00Z"},
  "seven_day_opus": null,
  "seven_day_sonnet": null,
  "limits": [
    {"kind": "session",        "group": "session", "percent": 21, "resets_at": "2026-09-17T17:30:00Z", "scope": null},
    {"kind": "weekly_all",     "group": "weekly",  "percent": 73, "resets_at": "2026-09-19T03:00:00Z", "scope": null},
    {"kind": "weekly_scoped",  "group": "weekly",  "percent": 98, "resets_at": "2026-09-19T03:00:00Z",
     "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null}}
  ]
}`

const payloadNoScope = `{
  "five_hour": {"utilization": 21.0, "resets_at": "2026-09-17T17:30:00Z"},
  "seven_day": {"utilization": 73.0, "resets_at": "2026-09-19T03:00:00Z"},
  "limits": [
    {"kind": "session",    "group": "session", "percent": 21, "resets_at": "2026-09-17T17:30:00Z", "scope": null},
    {"kind": "weekly_all", "group": "weekly",  "percent": 73, "resets_at": "2026-09-19T03:00:00Z", "scope": null}
  ]
}`

func TestAModelScopedLimitIsRead(t *testing.T) {
	rep, err := parseUsage([]byte(payloadScoped))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if rep.Scoped == nil {
		t.Fatal("no scoped limit read — the highest of the three gauges stays invisible (#840)")
	}
	if rep.Scoped.Label != "Fable" {
		t.Errorf("label = %q, want the model's display name", rep.Scoped.Label)
	}
	if rep.Scoped.Pct != 98 {
		t.Errorf("percent = %v, want 98", rep.Scoped.Pct)
	}
	if rep.Scoped.Reset != "2026-09-19T03:00:00Z" {
		t.Errorf("reset = %q, want the entry's own resets_at", rep.Scoped.Reset)
	}
	// The two windows vigie already showed must be untouched by the new read.
	if rep.FiveHourPct != 21 || rep.SevenDayPct != 73 {
		t.Errorf("the existing gauges changed: %v / %v", rep.FiveHourPct, rep.SevenDayPct)
	}
}

// Absent, not zero. A gauge permanently at nothing trains the eye to skip the place
// where the exception appears, which is why `hidden N` is shown only when it has a
// value.
func TestNoScopedLimitLeavesNothingToDraw(t *testing.T) {
	rep, err := parseUsage([]byte(payloadNoScope))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if rep.Scoped != nil {
		t.Errorf("a scoped limit was invented from a payload that carries none: %+v", rep.Scoped)
	}
}

// An older endpoint, or one that stops sending the array, must not break the two
// gauges that already work. The flat fields stay the source for those.
func TestAPayloadWithoutTheArrayStillFillsTheOldGauges(t *testing.T) {
	rep, err := parseUsage([]byte(`{"five_hour":{"utilization":21.0},"seven_day":{"utilization":73.0}}`))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if rep.FiveHourPct != 21 || rep.SevenDayPct != 73 || rep.Scoped != nil {
		t.Errorf("degraded payload handled wrongly: %+v", rep)
	}
}
