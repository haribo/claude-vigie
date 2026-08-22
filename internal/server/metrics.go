package server

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/haribo/claude-vigie/internal/clock"
	"github.com/haribo/claude-vigie/internal/status"
)

// This file is the daemon's only Prometheus dependency; the client packages
// never import it (ADR-0003, enforced by depguard). Labels are deliberately
// bounded — never session_id / machine / project — so cardinality stays finite.

const metricsNamespace = "vigie"

// modelFamilies are the label values vigie_output_tokens_total may take, besides
// "other". A family, not a model id: a version is what changes often, and every
// distinct label value is a counter Prometheus holds in memory for the process's
// lifetime and never frees.
//
// The model name arrives in the report body, so without this a client sending a
// different one each time grows the daemon without limit — no malice needed, a
// name carrying a date or an id does it by accident (#528).
//
// The per-version breakdown is not lost: stats_daily keeps the exact model, and
// the Stats tab reads it. That is what lets this be coarse.
var modelFamilies = []string{"opus", "sonnet", "haiku", "fable"}

// modelLabel folds a model id onto its family, or "other".
//
// "other" rather than dropping the sample: a counter that silently under-counts
// is worse than one with a bucket whose name says it is a catch-all, because the
// total still looks complete.
func modelLabel(model string) string {
	m := strings.ToLower(model)
	for _, f := range modelFamilies {
		if strings.Contains(m, f) {
			return f
		}
	}
	return "other"
}

var (
	// reg is the daemon's registry: Go/process collectors, the vigie_* metrics
	// below, and a scrape-time state collector the daemon registers separately.
	reg = prometheus.NewRegistry()

	metricHTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace, Name: "http_requests_total",
		Help: "API requests by route, method, and status code.",
	}, []string{"route", "method", "code"})

	metricHTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace, Name: "http_request_duration_seconds",
		Help:    "API request latency by route and method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})

	metricHTTPInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace, Name: "http_requests_in_flight",
		Help: "API requests currently being served.",
	})

	metricReports = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace, Name: "reports_total",
		Help: "Session event reports ingested, by event.",
	}, []string{"event"})

	metricReportsRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace, Name: "reports_rejected_total",
		Help: "Rejected reports, by reason.",
	}, []string{"reason"})

	metricOutputTokens = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: metricsNamespace, Name: "output_tokens_total",
		Help: "Output tokens ingested, by model.",
	}, []string{"model"})

	metricSSESubscribers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace, Name: "sse_subscribers",
		Help: "Current SSE subscribers.",
	})

	metricSSEPublished = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace, Name: "sse_events_published_total",
		Help: "SSE change notifications published.",
	})

	metricPruned = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace, Name: "sessions_pruned_total",
		Help: "Sessions garbage-collected by retention.",
	})

	metricBuildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace, Name: "build_info",
		Help: "Build metadata; value is always 1.",
	}, []string{"version", "go_version"})
)

func init() {
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		metricHTTPRequests, metricHTTPDuration, metricHTTPInFlight,
		metricReports, metricReportsRejected, metricOutputTokens,
		metricSSESubscribers, metricSSEPublished, metricPruned, metricBuildInfo,
		&stateCollector{},
	)
}

// MetricsHandler serves the Prometheus exposition (mounted on the ops listener).
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// SetBuildInfo records the vigie_build_info series.
func SetBuildInfo(version, goVersion string) {
	metricBuildInfo.WithLabelValues(version, goVersion).Set(1)
}

// IncPruned records sessions garbage-collected by the daemon's prune loop.
func IncPruned(n int) {
	if n > 0 {
		metricPruned.Add(float64(n))
	}
}

// withMetrics wraps the API mux with RED metrics. The route label is the mux
// pattern (matched after routing), never the raw URL, so cardinality is bounded.
func withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metricHTTPInFlight.Inc()
		defer metricHTTPInFlight.Dec()
		start := clock.Now()

		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := routeLabel(r.Pattern)
		metricHTTPRequests.WithLabelValues(route, r.Method, strconv.Itoa(rec.code)).Inc()
		// The SSE stream is long-lived; its "duration" is a connection lifetime,
		// not a latency, so keep it out of the request-latency histogram.
		if route != "/api/events" {
			metricHTTPDuration.WithLabelValues(route, r.Method).Observe(clock.Since(start).Seconds())
		}
	})
}

// routeLabel reduces a matched mux pattern ("POST /api/report") to its path, or
// "unmatched" when no route matched (bounded — never the raw URL).
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if i := strings.LastIndex(pattern, " "); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}

// statusRecorder captures the response status code while preserving the
// http.Flusher the SSE handler needs.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// RegisterStateCollector wires the scrape-time gauges (current sessions by
// reconciled status, DB size, watcher heartbeat) to a store. The collector
// itself is registered once at init; this just supplies the data source, so it
// is safe to call more than once (the daemon calls it exactly once).
func RegisterStateCollector(st Store, dbPath string) {
	stateMu.Lock()
	stateStore, stateDBPath = st, dbPath
	stateMu.Unlock()
}

var (
	stateMu     sync.RWMutex
	stateStore  Store
	stateDBPath string
)

// stateCollector emits the scrape-time gauges from the store supplied via
// RegisterStateCollector. It is inert until wired.
type stateCollector struct{}

var (
	descSessions = prometheus.NewDesc(metricsNamespace+"_sessions",
		"Current sessions by reconciled status.", []string{"status"}, nil)
	descDBSize = prometheus.NewDesc(metricsNamespace+"_db_size_bytes",
		"SQLite database file size.", nil, nil)
	descWatcher = prometheus.NewDesc(metricsNamespace+"_watcher_last_seen_timestamp_seconds",
		"Unix time of the last watch report received.", nil, nil)
)

// statusOrder is every status the gauge reports, so each series exists (=0) even
// when no session currently holds it. It comes from the shared vocabulary rather
// than a local copy: this list silently omitted `compacting` from the day it was
// added (#342), so those sessions were counted and then dropped, and the gauge's
// total did not match the number of sessions (#421, #423).
var statusOrder = status.All

func (c *stateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descSessions
	ch <- descDBSize
	ch <- descWatcher
}

func (c *stateCollector) Collect(ch chan<- prometheus.Metric) {
	stateMu.RLock()
	store, dbPath := stateStore, stateDBPath
	stateMu.RUnlock()
	if store == nil {
		return // not wired yet
	}

	ctx := context.Background()
	now := clock.Now()

	counts := map[string]int{}
	if sessions, err := store.ListSessions(ctx); err == nil {
		watched := watchedMachines(ctx, store, sessions, now)
		for _, s := range sessions {
			counts[toView(s, nil, now, watched[s.Machine]).Status]++ // reconciled status, matching the API/TUI
		}
	}
	for _, status := range statusOrder {
		ch <- prometheus.MustNewConstMetric(descSessions, prometheus.GaugeValue, float64(counts[status]), status)
	}

	if fi, err := os.Stat(dbPath); err == nil {
		ch <- prometheus.MustNewConstMetric(descDBSize, prometheus.GaugeValue, float64(fi.Size()))
	}
	if v, ok, err := store.GetMeta(ctx, watchSeenKey); err == nil && ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			ch <- prometheus.MustNewConstMetric(descWatcher, prometheus.GaugeValue, float64(t.Unix()))
		}
	}
}
