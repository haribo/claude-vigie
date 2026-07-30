package server

import "testing"

func TestMergeStatus(t *testing.T) {
	cases := []struct{ current, incoming, want string }{
		{"waiting", "idle", "waiting"},     // watcher idle must not clear waiting
		{"waiting", "working", "working"},  // real activity resumes
		{"waiting", "ended", "ended"},      // session ends
		{"idle", "working", "working"},     // normal case unaffected
		{"working", "idle", "working"},     // #190: working sticky over the watcher's idle
		{"working", "ended", "ended"},      // a dead/closed session still ends it
		{"working", "working", "working"},  // no-op
		{"thinking", "idle", "thinking"},   // #191: thinking is an active turn too
		{"thinking", "working", "working"}, // resumes to visible output
	}
	for _, c := range cases {
		if got := mergeStatus(c.current, c.incoming); got != c.want {
			t.Errorf("mergeStatus(%q,%q) = %q, want %q", c.current, c.incoming, got, c.want)
		}
	}
}
