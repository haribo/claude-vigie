package server

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/haribo/claude-fleet/internal/clock"
)

// This file is the daemon's only Prometheus dependency; the client packages
// never import it (ADR-0003, enforced by depguard). Labels are deliberately
// bounded — never session_id / machine / project — so cardinality stays finite.

const metricsNamespace = "fleet"

var (
	// reg is the daemon's registry: Go/process collectors, the fleet_* metrics
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
	)
}

// MetricsHandler serves the Prometheus exposition (mounted on the ops listener).
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// SetBuildInfo records the fleet_build_info series.
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
			metricHTTPDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
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

// RegisterStateCollector adds the scrape-time gauges (current sessions by
// reconciled status, DB size, watcher heartbeat) to the registry. Computed on
// scrape via ListSessions — no counter on the hot path.
func RegisterStateCollector(st Store, dbPath string) {
	reg.MustRegister(&stateCollector{store: st, dbPath: dbPath})
}

type stateCollector struct {
	store  Store
	dbPath string
}

var (
	descSessions = prometheus.NewDesc(metricsNamespace+"_sessions",
		"Current sessions by reconciled status.", []string{"status"}, nil)
	descDBSize = prometheus.NewDesc(metricsNamespace+"_db_size_bytes",
		"SQLite database file size.", nil, nil)
	descWatcher = prometheus.NewDesc(metricsNamespace+"_watcher_last_seen_timestamp_seconds",
		"Unix time of the last watch report received.", nil, nil)
)

// statusOrder is every status the gauge reports, so each series exists (=0) even
// when no session currently holds it.
var statusOrder = []string{"working", "thinking", "waiting", "idle", "error", "ended"}

func (c *stateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- descSessions
	ch <- descDBSize
	ch <- descWatcher
}

func (c *stateCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	now := clock.Now()

	counts := map[string]int{}
	if sessions, err := c.store.ListSessions(ctx); err == nil {
		for _, s := range sessions {
			counts[toView(s, nil, now).Status]++ // reconciled status, matching the API/TUI
		}
	}
	for _, status := range statusOrder {
		ch <- prometheus.MustNewConstMetric(descSessions, prometheus.GaugeValue, float64(counts[status]), status)
	}

	if fi, err := os.Stat(c.dbPath); err == nil {
		ch <- prometheus.MustNewConstMetric(descDBSize, prometheus.GaugeValue, float64(fi.Size()))
	}
	if v, ok, err := c.store.GetMeta(ctx, watchSeenKey); err == nil && ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			ch <- prometheus.MustNewConstMetric(descWatcher, prometheus.GaugeValue, float64(t.Unix()))
		}
	}
}
