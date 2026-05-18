// Package metrics provides observability counters, gauges, and health summaries
// for the AutoStack platform. All metrics are in-process and structured —
// no external Prometheus scrape endpoint is wired here (that belongs in the API layer).
//
// Invariant: metrics only observe state; they never modify it.
// Credentials and secret values never appear in metric labels or values.
package metrics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// MetricKind classifies a metric type.
type MetricKind string

const (
	KindCounter   MetricKind = "counter"
	KindGauge     MetricKind = "gauge"
	KindHistogram MetricKind = "histogram"
)

// MetricSample is a single observation at a point in time.
type MetricSample struct {
	Name        string
	Kind        MetricKind
	Value       float64
	Labels      map[string]string
	ObservedAt  string
}

// Counter is a monotonically increasing, thread-safe counter.
type Counter struct {
	name   string
	labels map[string]string
	n      atomic.Int64
}

// NewCounter creates a named counter.
func NewCounter(name string, labels map[string]string) *Counter {
	l := make(map[string]string, len(labels))
	for k, v := range labels {
		l[k] = v
	}
	return &Counter{name: name, labels: l}
}

// Inc increments the counter by 1.
func (c *Counter) Inc() { c.n.Add(1) }

// Add increments the counter by delta (must be non-negative).
func (c *Counter) Add(delta int64) { c.n.Add(delta) }

// Value returns the current counter value.
func (c *Counter) Value() int64 { return c.n.Load() }

// Sample returns a MetricSample snapshot.
func (c *Counter) Sample() MetricSample {
	return MetricSample{
		Name:       c.name,
		Kind:       KindCounter,
		Value:      float64(c.n.Load()),
		Labels:     c.labels,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// Gauge is a thread-safe value that can go up or down.
type Gauge struct {
	mu     sync.Mutex
	name   string
	labels map[string]string
	val    float64
}

// NewGauge creates a named gauge.
func NewGauge(name string, labels map[string]string) *Gauge {
	l := make(map[string]string, len(labels))
	for k, v := range labels {
		l[k] = v
	}
	return &Gauge{name: name, labels: l}
}

// Set sets the gauge to value.
func (g *Gauge) Set(value float64) {
	g.mu.Lock()
	g.val = value
	g.mu.Unlock()
}

// Value returns the current gauge value.
func (g *Gauge) Value() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.val
}

// Sample returns a MetricSample snapshot.
func (g *Gauge) Sample() MetricSample {
	return MetricSample{
		Name:       g.name,
		Kind:       KindGauge,
		Value:      g.Value(),
		Labels:     g.labels,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// Histogram tracks a distribution of observed values (e.g. latencies).
type Histogram struct {
	mu     sync.Mutex
	name   string
	labels map[string]string
	count  int64
	sum    float64
	min    float64
	max    float64
}

// NewHistogram creates a named histogram.
func NewHistogram(name string, labels map[string]string) *Histogram {
	l := make(map[string]string, len(labels))
	for k, v := range labels {
		l[k] = v
	}
	return &Histogram{name: name, labels: l}
}

// Observe records a single value.
func (h *Histogram) Observe(v float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	h.sum += v
	if h.count == 1 || v < h.min {
		h.min = v
	}
	if h.count == 1 || v > h.max {
		h.max = v
	}
}

// Stats returns (count, mean, min, max).
func (h *Histogram) Stats() (count int64, mean, min, max float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count == 0 {
		return 0, 0, 0, 0
	}
	return h.count, h.sum / float64(h.count), h.min, h.max
}

// Sample returns a MetricSample using the mean value.
func (h *Histogram) Sample() MetricSample {
	_, mean, _, _ := h.Stats()
	return MetricSample{
		Name:       h.name,
		Kind:       KindHistogram,
		Value:      mean,
		Labels:     h.labels,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// PlatformMetrics is the central registry of all AutoStack platform metrics.
// Wire this to the API layer to expose a /metrics endpoint.
type PlatformMetrics struct {
	// Execution metrics.
	TasksTotal        *Counter
	TasksCompleted    *Counter
	TasksFailed       *Counter
	TasksRetried      *Counter

	// Replay metrics.
	ReplayRequests    *Counter
	ReplaySuccesses   *Counter
	ReplayFailures    *Counter
	ReplayLatencyMs   *Histogram

	// Governance metrics.
	GovernanceApprovals *Counter
	GovernanceDenials   *Counter
	PendingApprovals    *Gauge

	// Contradiction metrics.
	ContradictionsDetected *Counter
	ContradictionsByType   map[string]*Counter

	// Worker metrics.
	WorkersActive     *Gauge
	WorkersQuarantined *Gauge
	WorkerAssignments *Counter

	// Security metrics.
	AuthSuccesses     *Counter
	AuthFailures      *Counter
	RateLimitHits     *Counter
	PermDenials       *Counter

	// Archive metrics.
	ArchiveEntries    *Gauge
	ForensicExports   *Counter

	// Certification metrics.
	CertificationRuns *Counter
	CertificationPass *Counter
	CertificationFail *Counter

	mu sync.Mutex
}

// NewPlatformMetrics creates a fully initialized metrics registry.
func NewPlatformMetrics() *PlatformMetrics {
	return &PlatformMetrics{
		TasksTotal:      NewCounter("autostack_tasks_total", nil),
		TasksCompleted:  NewCounter("autostack_tasks_completed_total", nil),
		TasksFailed:     NewCounter("autostack_tasks_failed_total", nil),
		TasksRetried:    NewCounter("autostack_tasks_retried_total", nil),

		ReplayRequests:  NewCounter("autostack_replay_requests_total", nil),
		ReplaySuccesses: NewCounter("autostack_replay_successes_total", nil),
		ReplayFailures:  NewCounter("autostack_replay_failures_total", nil),
		ReplayLatencyMs: NewHistogram("autostack_replay_latency_ms", nil),

		GovernanceApprovals: NewCounter("autostack_governance_approvals_total", nil),
		GovernanceDenials:   NewCounter("autostack_governance_denials_total", nil),
		PendingApprovals:    NewGauge("autostack_governance_pending_approvals", nil),

		ContradictionsDetected: NewCounter("autostack_contradictions_detected_total", nil),
		ContradictionsByType:   make(map[string]*Counter),

		WorkersActive:      NewGauge("autostack_workers_active", nil),
		WorkersQuarantined: NewGauge("autostack_workers_quarantined", nil),
		WorkerAssignments:  NewCounter("autostack_worker_assignments_total", nil),

		AuthSuccesses:  NewCounter("autostack_auth_successes_total", nil),
		AuthFailures:   NewCounter("autostack_auth_failures_total", nil),
		RateLimitHits:  NewCounter("autostack_rate_limit_hits_total", nil),
		PermDenials:    NewCounter("autostack_perm_denials_total", nil),

		ArchiveEntries:  NewGauge("autostack_archive_entries", nil),
		ForensicExports: NewCounter("autostack_forensic_exports_total", nil),

		CertificationRuns: NewCounter("autostack_certification_runs_total", nil),
		CertificationPass: NewCounter("autostack_certification_pass_total", nil),
		CertificationFail: NewCounter("autostack_certification_fail_total", nil),
	}
}

// RecordContradiction increments the contradiction counter for the given type.
func (m *PlatformMetrics) RecordContradiction(contradictionType string) {
	m.ContradictionsDetected.Inc()
	m.mu.Lock()
	c, ok := m.ContradictionsByType[contradictionType]
	if !ok {
		c = NewCounter(fmt.Sprintf("autostack_contradictions_%s_total", contradictionType), map[string]string{"type": contradictionType})
		m.ContradictionsByType[contradictionType] = c
	}
	m.mu.Unlock()
	c.Inc()
}

// Snapshot returns all current metric samples.
func (m *PlatformMetrics) Snapshot() []MetricSample {
	var samples []MetricSample
	samples = append(samples,
		m.TasksTotal.Sample(),
		m.TasksCompleted.Sample(),
		m.TasksFailed.Sample(),
		m.TasksRetried.Sample(),
		m.ReplayRequests.Sample(),
		m.ReplaySuccesses.Sample(),
		m.ReplayFailures.Sample(),
		m.ReplayLatencyMs.Sample(),
		m.GovernanceApprovals.Sample(),
		m.GovernanceDenials.Sample(),
		m.PendingApprovals.Sample(),
		m.ContradictionsDetected.Sample(),
		m.WorkersActive.Sample(),
		m.WorkersQuarantined.Sample(),
		m.WorkerAssignments.Sample(),
		m.AuthSuccesses.Sample(),
		m.AuthFailures.Sample(),
		m.RateLimitHits.Sample(),
		m.PermDenials.Sample(),
		m.ArchiveEntries.Sample(),
		m.ForensicExports.Sample(),
		m.CertificationRuns.Sample(),
		m.CertificationPass.Sample(),
		m.CertificationFail.Sample(),
	)
	m.mu.Lock()
	for _, c := range m.ContradictionsByType {
		samples = append(samples, c.Sample())
	}
	m.mu.Unlock()
	return samples
}

// HealthSummary is a human-readable operational health overview.
type HealthSummary struct {
	TotalTasks         int64
	CompletedTasks     int64
	FailedTasks        int64
	RetriedTasks       int64
	CompletionRate     float64 // completed / total, 0 if no tasks
	FailureRate        float64 // failed / total, 0 if no tasks
	Contradictions     int64
	ActiveWorkers      float64
	QuarantinedWorkers float64
	PendingApprovals   float64
	AuthFailures       int64
	CertificationsPassed int64
	Healthy            bool   // true if failure rate < 5% and no quarantined workers and auth failures < 10
	HealthReason       string
}

// BuildHealthSummary derives a health summary from the current metric state.
func (m *PlatformMetrics) BuildHealthSummary() HealthSummary {
	total := m.TasksTotal.Value()
	completed := m.TasksCompleted.Value()
	failed := m.TasksFailed.Value()
	retried := m.TasksRetried.Value()
	contradictions := m.ContradictionsDetected.Value()
	activeWorkers := m.WorkersActive.Value()
	quarantined := m.WorkersQuarantined.Value()
	pending := m.PendingApprovals.Value()
	authFail := m.AuthFailures.Value()
	certPass := m.CertificationPass.Value()

	var completionRate, failureRate float64
	if total > 0 {
		completionRate = float64(completed) / float64(total)
		failureRate = float64(failed) / float64(total)
	}

	healthy := true
	reason := "healthy"

	if failureRate >= 0.05 && total > 0 {
		healthy = false
		reason = fmt.Sprintf("failure rate %.1f%% exceeds 5%% threshold", failureRate*100)
	} else if quarantined > 0 {
		healthy = false
		reason = fmt.Sprintf("%.0f worker(s) quarantined", quarantined)
	} else if authFail >= 10 {
		healthy = false
		reason = fmt.Sprintf("%d auth failures detected", authFail)
	}

	return HealthSummary{
		TotalTasks:           total,
		CompletedTasks:       completed,
		FailedTasks:          failed,
		RetriedTasks:         retried,
		CompletionRate:       completionRate,
		FailureRate:          failureRate,
		Contradictions:       contradictions,
		ActiveWorkers:        activeWorkers,
		QuarantinedWorkers:   quarantined,
		PendingApprovals:     pending,
		AuthFailures:         authFail,
		CertificationsPassed: certPass,
		Healthy:              healthy,
		HealthReason:         reason,
	}
}
