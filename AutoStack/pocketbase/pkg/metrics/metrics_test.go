package metrics

import (
	"sync"
	"testing"
)

func TestCounter_IncAndValue(t *testing.T) {
	c := NewCounter("test_counter", map[string]string{"env": "test"})
	if c.Value() != 0 {
		t.Errorf("initial value should be 0, got %d", c.Value())
	}
	c.Inc()
	c.Inc()
	c.Add(3)
	if c.Value() != 5 {
		t.Errorf("expected 5, got %d", c.Value())
	}
}

func TestCounter_Sample(t *testing.T) {
	c := NewCounter("sample_counter", nil)
	c.Add(7)
	s := c.Sample()
	if s.Value != 7 {
		t.Errorf("sample value should be 7, got %f", s.Value)
	}
	if s.Kind != KindCounter {
		t.Errorf("expected counter kind, got %s", s.Kind)
	}
}

func TestGauge_SetAndValue(t *testing.T) {
	g := NewGauge("test_gauge", nil)
	g.Set(42.5)
	if g.Value() != 42.5 {
		t.Errorf("expected 42.5, got %f", g.Value())
	}
	g.Set(0)
	if g.Value() != 0 {
		t.Errorf("expected 0 after reset, got %f", g.Value())
	}
}

func TestGauge_ConcurrentSafe(t *testing.T) {
	g := NewGauge("concurrent_gauge", nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n float64) {
			defer wg.Done()
			g.Set(n)
		}(float64(i))
	}
	wg.Wait()
	// Just verify no race — exact value is non-deterministic.
	_ = g.Value()
}

func TestHistogram_ObserveAndStats(t *testing.T) {
	h := NewHistogram("test_latency", nil)
	h.Observe(10)
	h.Observe(20)
	h.Observe(30)
	count, mean, min, max := h.Stats()
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
	if mean != 20 {
		t.Errorf("expected mean 20, got %f", mean)
	}
	if min != 10 {
		t.Errorf("expected min 10, got %f", min)
	}
	if max != 30 {
		t.Errorf("expected max 30, got %f", max)
	}
}

func TestHistogram_EmptyStats(t *testing.T) {
	h := NewHistogram("empty_hist", nil)
	count, mean, min, max := h.Stats()
	if count != 0 || mean != 0 || min != 0 || max != 0 {
		t.Errorf("empty histogram should return zeros: count=%d mean=%f min=%f max=%f", count, mean, min, max)
	}
}

func TestPlatformMetrics_NewIsZero(t *testing.T) {
	m := NewPlatformMetrics()
	if m.TasksTotal.Value() != 0 {
		t.Error("fresh metrics should start at zero")
	}
	if m.WorkersActive.Value() != 0 {
		t.Error("fresh gauge should start at zero")
	}
}

func TestPlatformMetrics_RecordContradiction(t *testing.T) {
	m := NewPlatformMetrics()
	m.RecordContradiction("deploy-missing")
	m.RecordContradiction("deploy-missing")
	m.RecordContradiction("delete-alive")

	if m.ContradictionsDetected.Value() != 3 {
		t.Errorf("expected 3 total contradictions, got %d", m.ContradictionsDetected.Value())
	}
	c, ok := m.ContradictionsByType["deploy-missing"]
	if !ok {
		t.Fatal("deploy-missing counter not created")
	}
	if c.Value() != 2 {
		t.Errorf("expected 2 deploy-missing, got %d", c.Value())
	}
}

func TestPlatformMetrics_Snapshot(t *testing.T) {
	m := NewPlatformMetrics()
	m.TasksTotal.Add(100)
	m.TasksCompleted.Add(90)
	m.TasksFailed.Add(10)
	m.RecordContradiction("scale-unchanged")

	samples := m.Snapshot()
	if len(samples) == 0 {
		t.Fatal("snapshot should not be empty")
	}
	// Verify named counters appear.
	found := false
	for _, s := range samples {
		if s.Name == "autostack_tasks_total" && s.Value == 100 {
			found = true
			break
		}
	}
	if !found {
		t.Error("tasks_total not found in snapshot with correct value")
	}
}

func TestHealthSummary_Healthy(t *testing.T) {
	m := NewPlatformMetrics()
	m.TasksTotal.Add(100)
	m.TasksCompleted.Add(98)
	m.TasksFailed.Add(2) // 2% failure — below 5% threshold
	m.WorkersActive.Set(3)
	m.CertificationPass.Add(1)

	h := m.BuildHealthSummary()
	if !h.Healthy {
		t.Errorf("should be healthy: %s", h.HealthReason)
	}
	if h.CompletionRate < 0.97 || h.CompletionRate > 0.99 {
		t.Errorf("unexpected completion rate: %f", h.CompletionRate)
	}
}

func TestHealthSummary_UnhealthyHighFailureRate(t *testing.T) {
	m := NewPlatformMetrics()
	m.TasksTotal.Add(100)
	m.TasksCompleted.Add(90)
	m.TasksFailed.Add(10) // 10% — above 5% threshold

	h := m.BuildHealthSummary()
	if h.Healthy {
		t.Error("should be unhealthy with 10% failure rate")
	}
}

func TestHealthSummary_UnhealthyQuarantinedWorkers(t *testing.T) {
	m := NewPlatformMetrics()
	m.TasksTotal.Add(10)
	m.TasksCompleted.Add(10)
	m.WorkersQuarantined.Set(1)

	h := m.BuildHealthSummary()
	if h.Healthy {
		t.Error("should be unhealthy with quarantined workers")
	}
}

func TestHealthSummary_UnhealthyAuthFailures(t *testing.T) {
	m := NewPlatformMetrics()
	m.AuthFailures.Add(10) // exactly 10 — triggers threshold

	h := m.BuildHealthSummary()
	if h.Healthy {
		t.Error("should be unhealthy with >=10 auth failures")
	}
}

func TestHealthSummary_ZeroTasks(t *testing.T) {
	m := NewPlatformMetrics()
	h := m.BuildHealthSummary()
	if h.CompletionRate != 0 || h.FailureRate != 0 {
		t.Error("zero tasks should produce zero rates")
	}
	if !h.Healthy {
		t.Errorf("fresh system should be healthy: %s", h.HealthReason)
	}
}

func TestCounter_LabelsCopied(t *testing.T) {
	labels := map[string]string{"env": "prod"}
	c := NewCounter("lbl_counter", labels)
	labels["env"] = "mutated"
	s := c.Sample()
	if s.Labels["env"] == "mutated" {
		t.Error("counter labels must be copied, not shared with caller")
	}
}
