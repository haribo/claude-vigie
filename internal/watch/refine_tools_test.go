package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// An outstanding tool call means Claude is waiting on a command, and that is
// `working` — for as long as it takes. The table used to assert a threshold past
// which the same observation became `stalled`; ADR-0012 removed that verdict,
// because the pairing proves a call is outstanding and never that it is hung.
func TestRefineWithTools(t *testing.T) {
	long := time.Hour
	cases := []struct {
		name string
		base string
		info transcript.Info
		age  time.Duration
		want string
	}{
		{"idle + an outstanding tool → working, whatever the age", "idle", transcript.Info{PendingTool: "Bash"}, long, "working"},
		{"idle + an outstanding tool, freshly started → working", "idle", transcript.Info{PendingTool: "Bash"}, 5 * time.Second, "working"},
		{"idle + running background task → working", "idle", transcript.Info{BackgroundActive: true}, long, "working"},
		{"idle + nothing pending → idle", "idle", transcript.Info{}, long, "idle"},
		{"working is never reclassified", "working", transcript.Info{PendingTool: "Bash"}, long, "working"},
		{"waiting is never reclassified", "waiting", transcript.Info{PendingTool: "Bash"}, long, "waiting"},
		{"ended is never reclassified", "ended", transcript.Info{BackgroundActive: true}, long, "ended"},
	}
	for _, c := range cases {
		info := c.info
		if got := refineWithTools(c.base, &info, c.age); got != c.want {
			t.Errorf("%s: refineWithTools(%q) = %q, want %q", c.name, c.base, got, c.want)
		}
	}
}
