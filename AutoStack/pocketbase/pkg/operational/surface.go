// Package operational wires the live operational surface: execution timelines,
// contradiction alerts, approval queues, certification state, and worker health.
// All views are derived read-only projections — they are never source-of-truth.
package operational

import (
	"time"

	"github.com/janlauber/one-click/pkg/audit"
	"github.com/janlauber/one-click/pkg/executor"
	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/observability"
	"github.com/janlauber/one-click/pkg/providerops"
	"github.com/janlauber/one-click/pkg/workers"
)

// OperationalSurface is the unified read-only view of a tenant's live runtime state.
// It is rebuilt on every query — it is never persisted as source-of-truth.
type OperationalSurface struct {
	TenantID              string                            `json:"tenant_id"`
	ExecutionID           string                            `json:"execution_id"`
	TaskSummary           TaskSummary                       `json:"task_summary"`
	WorkerHealth          WorkerHealthView                  `json:"worker_health"`
	ContradictionAlert    ContradictionAlertView            `json:"contradiction_alert"`
	ApprovalQueue         []governance.ApprovalStep         `json:"approval_queue"`
	CertificationState    CertificationStateView            `json:"certification_state"`
	TimelineEntries       []observability.TimelineEntry     `json:"timeline_entries"`
	GeneratedAt           string                            `json:"generated_at"`
}

// TaskSummary is a compact count of task states for display.
type TaskSummary struct {
	Total    int            `json:"total"`
	ByState  map[string]int `json:"by_state"`
	Terminal int            `json:"terminal"`
}

// WorkerHealthView is a compact read-only view of worker fleet health.
type WorkerHealthView struct {
	Total       int            `json:"total"`
	Online      int            `json:"online"`
	Degraded    int            `json:"degraded"`
	Quarantined int            `json:"quarantined"`
	Offline     int            `json:"offline"`
}

// ContradictionAlertView is a read-only alert summary for an execution.
type ContradictionAlertView struct {
	HasContradictions bool   `json:"has_contradictions"`
	TotalCount        int    `json:"total_count"`
	CriticalCount     int    `json:"critical_count"`
	HighestSeverity   string `json:"highest_severity,omitempty"`
}

// CertificationStateView summarizes the certification status of an execution.
type CertificationStateView struct {
	HasCertification bool   `json:"has_certification"`
	Certified        bool   `json:"certified"`
	CertificationID  string `json:"certification_id,omitempty"`
	CertifiedAt      string `json:"certified_at,omitempty"`
}

// BuildOperationalSurface assembles the full operational surface for one execution.
// All inputs are read-only; nothing is mutated.
func BuildOperationalSurface(
	executionID, tenantID string,
	tasks []executor.ExecutionTask,
	allWorkers []workers.Worker,
	contradictions []providerops.ProviderContradiction,
	contraSum *observability.ContradictionSummary,
	hierarchy *governance.ApprovalHierarchy,
	certs []audit.ReplayCertificationReport,
	timeline observability.ObservabilityTimeline,
) OperationalSurface {
	surface := OperationalSurface{
		TenantID:    tenantID,
		ExecutionID: executionID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	// Task summary.
	ts := TaskSummary{ByState: make(map[string]int)}
	for _, t := range tasks {
		if t.ExecutionID != executionID || t.TenantID != tenantID {
			continue
		}
		ts.Total++
		ts.ByState[string(t.State)]++
		if executor.IsTerminalTask(t) {
			ts.Terminal++
		}
	}
	surface.TaskSummary = ts

	// Worker health.
	wh := WorkerHealthView{}
	for _, w := range allWorkers {
		if w.TenantID != tenantID {
			continue
		}
		wh.Total++
		switch w.State {
		case workers.WorkerStateOnline:
			wh.Online++
		case workers.WorkerStateDegraded:
			wh.Degraded++
		case workers.WorkerStateQuarantined:
			wh.Quarantined++
		case workers.WorkerStateOffline:
			wh.Offline++
		}
	}
	surface.WorkerHealth = wh

	// Contradiction alert.
	ca := ContradictionAlertView{}
	for _, c := range contradictions {
		if c.ExecutionID != executionID || c.TenantID != tenantID {
			continue
		}
		ca.TotalCount++
	}
	ca.HasContradictions = ca.TotalCount > 0
	if contraSum != nil && ca.HasContradictions {
		ca.HighestSeverity = string(contraSum.HighestSeverity)
		if contraSum.HighestSeverity == observability.ContradictionSeverityCritical {
			ca.CriticalCount = ca.TotalCount // all counted contradictions are at critical level
		}
	}
	surface.ContradictionAlert = ca

	// Approval queue.
	if hierarchy != nil {
		surface.ApprovalQueue = governance.PendingSteps(*hierarchy)
	}

	// Certification state.
	cs := CertificationStateView{}
	var latestCert *audit.ReplayCertificationReport
	for i := range certs {
		if certs[i].ExecutionID != executionID || certs[i].TenantID != tenantID {
			continue
		}
		if latestCert == nil || certs[i].CertifiedAt > latestCert.CertifiedAt {
			cp := certs[i]
			latestCert = &cp
		}
	}
	if latestCert != nil {
		cs.HasCertification = true
		cs.Certified = latestCert.Certified
		cs.CertificationID = latestCert.CertificationID
		cs.CertifiedAt = latestCert.CertifiedAt
	}
	surface.CertificationState = cs

	// Timeline entries (copy, no mutation) — only include entries for this execution.
	if timeline.ExecutionID == executionID && timeline.TenantID == tenantID {
		surface.TimelineEntries = append(surface.TimelineEntries, timeline.Entries...)
	}

	return surface
}
