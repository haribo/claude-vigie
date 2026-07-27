package server

import "testing"

func TestMergeStatus(t *testing.T) {
	cases := []struct{ current, incoming, want string }{
		{"waiting", "idle", "waiting"},    // watcher idle must not clear waiting
		{"waiting", "working", "working"}, // real activity resumes
		{"waiting", "ended", "ended"},     // session ends
		{"idle", "working", "working"},    // normal case unaffected
		{"working", "idle", "idle"},       // normal downgrade unaffected
	}
	for _, c := range cases {
		if got := mergeStatus(c.current, c.incoming); got != c.want {
			t.Errorf("mergeStatus(%q,%q) = %q, want %q", c.current, c.incoming, got, c.want)
		}
	}
}
