// Package loadtest provides in-process load scenarios for AutoStack's core subsystems.
// All load is simulated — no real infrastructure, network, or cloud APIs are called.
// Results measure whether invariants hold under volume, not wall-clock throughput.
package loadtest

import (
	"fmt"
	"sync"
	"time"
)

// LoadScenario describes a single load test run.
type LoadScenario struct {
	Name        string
	Concurrency int
	Iterations  int
	Description string
}

// LoadResult is the outcome of a load scenario.
type LoadResult struct {
	ScenarioName  string
	TotalOps      int
	Succeeded     int
	Failed        int
	ErrorRate     float64
	DurationMs    int64
	InvariantHeld bool // key correctness invariant was never violated
	Violations    []string
	Notes         []string
}

// LoadRunner executes load scenarios and collects results.
type LoadRunner struct {
	results []LoadResult
}

// NewLoadRunner creates a fresh runner.
func NewLoadRunner() *LoadRunner {
	return &LoadRunner{}
}

// Run executes the scenario by calling workFn concurrently.
// workFn receives a worker index and iteration index; returns (succeeded bool, violation string).
func (r *LoadRunner) Run(scenario LoadScenario, workFn func(worker, iter int) (bool, string)) LoadResult {
	start := time.Now()
	type outcome struct {
		ok        bool
		violation string
	}

	ch := make(chan outcome, scenario.Concurrency*scenario.Iterations)
	var wg sync.WaitGroup

	for w := 0; w < scenario.Concurrency; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < scenario.Iterations; i++ {
				ok, v := workFn(worker, i)
				ch <- outcome{ok: ok, violation: v}
			}
		}(w)
	}
	wg.Wait()
	close(ch)

	total := scenario.Concurrency * scenario.Iterations
	succeeded := 0
	var violations []string
	for o := range ch {
		if o.ok {
			succeeded++
		}
		if o.violation != "" {
			violations = append(violations, o.violation)
		}
	}

	failed := total - succeeded
	errorRate := 0.0
	if total > 0 {
		errorRate = float64(failed) / float64(total)
	}

	result := LoadResult{
		ScenarioName:  scenario.Name,
		TotalOps:      total,
		Succeeded:     succeeded,
		Failed:        failed,
		ErrorRate:     errorRate,
		DurationMs:    time.Since(start).Milliseconds(),
		InvariantHeld: len(violations) == 0,
		Violations:    violations,
		Notes: []string{
			fmt.Sprintf("concurrency=%d iterations=%d", scenario.Concurrency, scenario.Iterations),
		},
	}
	r.results = append(r.results, result)
	return result
}

// AllResults returns a copy of all results.
func (r *LoadRunner) AllResults() []LoadResult {
	out := make([]LoadResult, len(r.results))
	copy(out, r.results)
	return out
}

// Summary returns a human-readable summary of all results.
func (r *LoadRunner) Summary() string {
	passed := 0
	for _, res := range r.results {
		if res.InvariantHeld {
			passed++
		}
	}
	return fmt.Sprintf("load test summary: %d/%d scenarios passed invariants", passed, len(r.results))
}
