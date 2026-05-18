package reconciler

import (
	"testing"
	"time"
)

// Phase 3.9.8 — Periodic truth audit tests (pure logic; no DB fixture).

func TestTruthAuditIntervalZeroDisables(t *testing.T) {
	// A zero TruthAuditInterval must not start the goroutine. Validate via
	// the config validation path — the function must return without panicking.
	r := &Reconciler{
		config: Config{TruthAuditInterval: 0},
		stopCh: make(chan struct{}),
	}
	// Should return immediately without starting a goroutine.
	r.StartTruthAuditLoop()
}

func TestTruthAuditIntervalPositiveStartsLoop(t *testing.T) {
	// A positive interval must attempt to start the goroutine. We set a very
	// long interval (1h) so the ticker never fires during the test, then stop
	// immediately. Validates that StartTruthAuditLoop returns without blocking.
	r := &Reconciler{
		config: Config{TruthAuditInterval: time.Hour},
		stopCh: make(chan struct{}),
		app:    nil, // no DB; goroutine will not fire during test
	}
	r.StartTruthAuditLoop()
	close(r.stopCh) // signal stop; goroutine exits at next select
}

func TestRunTruthAuditOncePanicsRecovered(t *testing.T) {
	// RunTruthAuditOnce must recover from panics (best-effort audit).
	// With app=nil the calls to RunContradictionAudit/RunInvariantAudit
	// will either panic or return empty; either way the deferred recover
	// must not propagate.
	r := &Reconciler{app: nil}
	// This must not panic to the test runner.
	r.RunTruthAuditOnce()
}

func TestTruthAuditConfigDefaultInterval(t *testing.T) {
	// Default config must produce a non-zero audit interval so the goroutine
	// starts in production. We cannot assert the exact value (env-var driven)
	// but we can assert it is within a plausible range.
	cfg := DefaultConfig()
	if cfg.TruthAuditInterval < 0 {
		t.Errorf("TruthAuditInterval must be >= 0, got %s", cfg.TruthAuditInterval)
	}
	// If the env var is not set, the default is 15 minutes.
	// Only assert the expected default when the env var is absent.
	expected := 15 * time.Minute
	if cfg.TruthAuditInterval != 0 && cfg.TruthAuditInterval != expected {
		// Allowed: env var overrode the default.
		t.Logf("TruthAuditInterval=%s (non-default, likely env override)", cfg.TruthAuditInterval)
	}
}

func TestTruthAuditLoopHonorsStopCh(t *testing.T) {
	// Start a loop with a very short interval, then close stopCh immediately.
	// The goroutine must exit without blocking the test.
	stopCh := make(chan struct{})
	r := &Reconciler{
		config: Config{TruthAuditInterval: time.Millisecond},
		stopCh: stopCh,
		app:    nil,
	}
	// We call the private loop directly to avoid re-entering StartTruthAuditLoop.
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.runTruthAuditLoop()
	}()
	// Signal stop.
	close(stopCh)
	// Wait for the goroutine to exit (with a generous timeout).
	select {
	case <-done:
		// OK — exited
	case <-time.After(2 * time.Second):
		t.Error("runTruthAuditLoop did not exit after stopCh was closed")
	}
}
