package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

func TestRefineWithTools(t *testing.T) {
	long := stalledAfter + time.Second
	cases := []struct {
		name string
		base string
		info transcript.Info
		age  time.Duration
		want string
	}{
		{"idle + hung foreground tool + inactive → stalled", "idle", transcript.Info{PendingTool: "Bash"}, long, "stalled"},
		{"idle + hung tool but still fresh → idle", "idle", transcript.Info{PendingTool: "Bash"}, 5 * time.Second, "idle"},
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
