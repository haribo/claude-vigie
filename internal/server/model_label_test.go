package server

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// #528. The model name arrives in the report body and became a Prometheus label
// unchecked. Prometheus creates a counter per distinct label value and never
// frees it, so a client sending a different name each time grows the daemon's
// memory without limit — no malice needed, a name carrying a date or an id does
// it by accident.
//
// The per-version breakdown lives in stats_daily, which the Stats tab reads, so
// the metric can afford to be coarse.

// seriesFor counts the series a CounterVec currently holds.
func seriesFor(t *testing.T, vec *prometheus.CounterVec) int {
	t.Helper()
	ch := make(chan prometheus.Metric, 4096)
	go func() {
		vec.Collect(ch)
		close(ch)
	}()
	n := 0
	for m := range ch {
		var d dto.Metric
		if err := m.Write(&d); err != nil {
			t.Fatal(err)
		}
		n++
	}
	return n
}

// The cardinality assertion: many distinct names must not become many series.
// Counting series is what makes this a cardinality test — asserting one string
// would pass against the unbounded version.
func TestManyModelNamesStayBounded(t *testing.T) {
	vec := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "t_bounded"}, []string{"model"})
	for i := 0; i < 500; i++ {
		vec.WithLabelValues(modelLabel(fmt.Sprintf("claude-opus-4-8-2025%04d", i))).Inc()
		vec.WithLabelValues(modelLabel(fmt.Sprintf("junk-%d", i))).Inc()
	}
	if got := seriesFor(t, vec); got > 8 {
		t.Errorf("1000 distinct model names produced %d series — the label is still unbounded", got)
	}
}

// The families a reader expects to tell apart must not all collapse into one
// bucket: a bound that answers "other" for everything is bounded and useless.
func TestTheFamiliesStayDistinct(t *testing.T) {
	seen := map[string]string{}
	for _, m := range []string{
		"claude-opus-4-8", "claude-opus-5", "claude-sonnet-4-5-20250929",
		"claude-haiku-4-5-20251001", "claude-fable-5",
	} {
		seen[m] = modelLabel(m)
	}
	distinct := map[string]bool{}
	for _, v := range seen {
		distinct[v] = true
	}
	if len(distinct) != 4 {
		t.Errorf("the four families collapsed into %d labels: %v", len(distinct), seen)
	}
	if modelLabel("claude-opus-4-8") != modelLabel("claude-opus-5") {
		t.Error("two versions of one family produced different labels — that is the cardinality this bounds")
	}
}

// An unrecognized name is counted, not dropped. Dropping would make the counter
// silently under-count, and a total that looks complete and is not is worse than
// a bucket named "other".
func TestAnUnknownModelIsCountedAsOther(t *testing.T) {
	for _, m := range []string{"gpt-4", "", "llama-3", "claude-", "claude-quantum-9"} {
		if got := modelLabel(m); got != "other" {
			t.Errorf("modelLabel(%q) = %q, want other", m, got)
		}
	}
}
