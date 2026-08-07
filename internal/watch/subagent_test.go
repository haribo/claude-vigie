package watch

import (
	"testing"
	"time"

	"github.com/haribo/claude-vigie/internal/transcript"
)

// TestRefineWithToolsAgents is the #344 regression at the watcher: an idle base
// with an in-flight subagent reads working (like a backgrounded Bash), and the
// liveness cap self-heals a missed close once the parent has been quiet past
// agentWindow.
func TestRefineWithToolsAgents(t *testing.T) {
	cases := []struct {
		name string
		base string
		info transcript.Info
		age  time.Duration
		want string
	}{
		{"idle + in-flight agent, fresh → working", "idle", transcript.Info{AgentsActive: 1}, 30 * time.Second, "working"},
		{"idle + in-flight agent, past the cap → idle (self-heal)", "idle", transcript.Info{AgentsActive: 1}, agentWindow + time.Minute, "idle"},
		{"idle + no agent → idle", "idle", transcript.Info{}, 30 * time.Second, "idle"},
		{"working is never reclassified by agents", "working", transcript.Info{AgentsActive: 3}, 30 * time.Second, "working"},
	}
	for _, c := range cases {
		info := c.info
		if got := refineWithTools(c.base, &info, c.age); got != c.want {
			t.Errorf("%s: refineWithTools(%q) = %q, want %q", c.name, c.base, got, c.want)
		}
	}
}
