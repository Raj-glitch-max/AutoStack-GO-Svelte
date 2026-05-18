package platformapi

import (
	"time"

	"github.com/janlauber/one-click/pkg/workers"
)

// WorkerView is a read-only projection of a Worker safe for API responses.
type WorkerView struct {
	WorkerID     string               `json:"worker_id"`
	TenantID     string               `json:"tenant_id"`
	State        workers.WorkerState  `json:"state"`
	Capabilities []string             `json:"capabilities"`
	Region       string               `json:"region"`
	HashValid    bool                 `json:"hash_valid"`
	RegisteredAt string               `json:"registered_at"`
}

// WorkerQueryFilter constrains which workers are returned.
type WorkerQueryFilter struct {
	TenantID     string
	State        workers.WorkerState // zero-value = all states
	Capability   string              // zero-value = all capabilities
}

// WorkerQueryResult groups worker views returned by a query.
type WorkerQueryResult struct {
	Workers     []WorkerView `json:"workers"`
	TotalCount  int          `json:"total_count"`
	LiveCount   int          `json:"live_count"`
	QueryAt     string       `json:"query_at"`
}

// QueryWorkers returns filtered, read-only views of workers.
// Hash integrity is verified per worker; invalid workers are flagged but included.
func QueryWorkers(allWorkers []workers.Worker, filter WorkerQueryFilter) WorkerQueryResult {
	result := WorkerQueryResult{
		QueryAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	for _, w := range allWorkers {
		if filter.TenantID != "" && w.TenantID != filter.TenantID {
			continue
		}
		if filter.State != "" && w.State != filter.State {
			continue
		}
		if filter.Capability != "" && !workers.HasCapability(w, filter.Capability) {
			continue
		}

		ok, _ := workers.VerifyWorkerHash(w)
		caps := make([]string, len(w.Capabilities))
		copy(caps, w.Capabilities)

		view := WorkerView{
			WorkerID:     w.WorkerID,
			TenantID:     w.TenantID,
			State:        w.State,
			Capabilities: caps,
			Region:       w.Region,
			HashValid:    ok,
			RegisteredAt: w.RegisteredAt,
		}
		result.Workers = append(result.Workers, view)
		if workers.IsAvailable(w) {
			result.LiveCount++
		}
	}

	result.TotalCount = len(result.Workers)
	return result
}

// WorkerHealthSummary gives aggregate health counts for a tenant.
type WorkerHealthSummary struct {
	TenantID    string         `json:"tenant_id"`
	StateCounts map[string]int `json:"state_counts"`
	TotalWorkers int           `json:"total_workers"`
	SummarizedAt string        `json:"summarized_at"`
}

// SummarizeWorkerHealth aggregates worker state counts for a tenant.
func SummarizeWorkerHealth(allWorkers []workers.Worker, tenantID string) WorkerHealthSummary {
	summary := WorkerHealthSummary{
		TenantID:     tenantID,
		StateCounts:  make(map[string]int),
		SummarizedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, w := range allWorkers {
		if w.TenantID != tenantID {
			continue
		}
		summary.StateCounts[string(w.State)]++
		summary.TotalWorkers++
	}
	return summary
}
