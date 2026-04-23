package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds all Prometheus metrics for the chapter-overview service.
type Registry struct {
	TasksTotal          *prometheus.CounterVec
	TokensTotal         *prometheus.CounterVec
	PagesProcessedTotal prometheus.Counter
	TaskDurationSeconds prometheus.Histogram
	LLMCallDurationMs   prometheus.Histogram
	ActiveTasks         prometheus.Gauge
	reg                 *prometheus.Registry
}

// New creates and registers all metrics.
func New() *Registry {
	reg := prometheus.NewRegistry()
	r := &Registry{
		TasksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "chapter_overview_tasks_total",
			Help: "Total tasks by terminal status.",
		}, []string{"status"}),
		TokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "chapter_overview_tokens_total",
			Help: "Total LLM tokens consumed, by type (input|output).",
		}, []string{"type"}),
		PagesProcessedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "chapter_overview_pages_processed_total",
			Help: "Total PDF pages processed across all tasks.",
		}),
		TaskDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "chapter_overview_task_duration_seconds",
			Help:    "End-to-end task duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		LLMCallDurationMs: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "chapter_overview_llm_call_duration_ms",
			Help:    "Individual LLM API call duration in milliseconds.",
			Buckets: []float64{100, 500, 1000, 2000, 5000, 10000, 30000},
		}),
		ActiveTasks: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "chapter_overview_active_tasks",
			Help: "Number of tasks currently being processed.",
		}),
		reg: reg,
	}
	reg.MustRegister(
		r.TasksTotal,
		r.TokensTotal,
		r.PagesProcessedTotal,
		r.TaskDurationSeconds,
		r.LLMCallDurationMs,
		r.ActiveTasks,
	)
	return r
}

// Handler returns an HTTP handler for the /metrics endpoint.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}
