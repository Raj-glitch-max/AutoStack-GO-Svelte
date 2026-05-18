// Package api exposes the operational platform surfaces as HTTP read-only
// query endpoints on the PocketBase Echo instance.
//
// All endpoints in this package are read-only — they assemble and return
// state derived from the underlying governance, scheduler, topology, verifier,
// and planner packages. They do NOT mutate any execution state. Mutations
// remain the exclusive responsibility of pkg/executor/api.
//
// Operator authority contract (inherited from CLAUDE.md):
//   - No endpoint triggers automatic rollback, compensation, or re-execution.
//   - No auto-approval, no silent escalation.
//   - Every override must be recorded via the governance package before it is
//     visible here.
package api

import (
	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/scheduler"
	"github.com/janlauber/one-click/pkg/topology"
	"github.com/janlauber/one-click/pkg/verifier"
)

// ─── Overview ──────────────────────────────────────────────────────────────

// PlatformOverview is the aggregate health summary returned by GET /platform/overview.
type PlatformOverview struct {
	ActiveExecutions   int                    `json:"active_executions"`
	VerifiedCount      int                    `json:"verified_count"`
	ContradictedCount  int                    `json:"contradicted_count"`
	BlockedStageCount  int                    `json:"blocked_stage_count"`
	PendingApprovals   int                    `json:"pending_approvals"`
	OverrideCount      int                    `json:"override_count"`
	Limitations        []string               `json:"limitations"`
	GeneratedAt        string                 `json:"generated_at"`
}

// ─── Executions ────────────────────────────────────────────────────────────

// ExecutionSummary is one row in the execution explorer list.
type ExecutionSummary struct {
	ExecutionID        string `json:"execution_id"`
	VerificationState  string `json:"verification_state"`
	ConfidenceScore    float64 `json:"confidence_score"`
	TotalStages        int    `json:"total_stages"`
	CompletedStages    int    `json:"completed_stages"`
	AuditEntryCount    int    `json:"audit_entry_count"`
	HasContradictions  bool   `json:"has_contradictions"`
	HasOverrides       bool   `json:"has_overrides"`
}

// ExecutionDetailResponse is returned by GET /platform/executions/:id.
type ExecutionDetailResponse struct {
	ExecutionID       string                          `json:"execution_id"`
	VerificationSnap  *verifier.VerificationSnapshot  `json:"verification_snapshot,omitempty"`
	GovernanceSnap    *governance.GovernanceSnapshot  `json:"governance_snapshot,omitempty"`
	SchedulingSnap    *scheduler.SchedulingSnapshot   `json:"scheduling_snapshot,omitempty"`
	TopologySnap      *topology.TopologySnapshot      `json:"topology_snapshot,omitempty"`
	Limitations       []string                        `json:"limitations"`
}

// ─── Topology ──────────────────────────────────────────────────────────────

// TopologyResponse is returned by GET /platform/topology/:id.
type TopologyResponse struct {
	ExecutionID     string                              `json:"execution_id"`
	Snapshot        *topology.TopologySnapshot          `json:"snapshot"`
	Explanation     *topology.TopologyExplanation       `json:"explanation"`
	Limitations     []string                            `json:"limitations"`
}

// ─── Scheduler ─────────────────────────────────────────────────────────────

// SchedulerResponse is returned by GET /platform/scheduler/:id.
type SchedulerResponse struct {
	ExecutionID  string                        `json:"execution_id"`
	Snapshot     *scheduler.SchedulingSnapshot `json:"snapshot"`
	Limitations  []string                      `json:"limitations"`
}

// ─── Governance ────────────────────────────────────────────────────────────

// GovernanceResponse is returned by GET /platform/governance/:id.
type GovernanceResponse struct {
	ExecutionID  string                          `json:"execution_id"`
	Snapshot     *governance.GovernanceSnapshot  `json:"snapshot"`
	ChainValid   bool                            `json:"chain_valid"`
	ChainReason  string                          `json:"chain_reason,omitempty"`
	Limitations  []string                        `json:"limitations"`
}

// ─── Replay ────────────────────────────────────────────────────────────────

// ReplayResponse is returned by GET /platform/replay/:id.
type ReplayResponse struct {
	ExecutionID  string                          `json:"execution_id"`
	ReplayOrder  []string                        `json:"replay_order"`
	Events       []scheduler.SchedulerReplayEvent `json:"events"`
	Limitations  []string                        `json:"limitations"`
}

// ─── Exports ───────────────────────────────────────────────────────────────

// ExportVerificationResponse is returned by POST /platform/exports/verify.
type ExportVerificationResponse struct {
	VerificationValid  bool   `json:"verification_valid"`
	GovernanceValid    bool   `json:"governance_valid"`
	TopologyValid      bool   `json:"topology_valid"`
	SchedulerValid     bool   `json:"scheduler_valid"`
	Reason             string `json:"reason,omitempty"`
}

// ExportVerificationRequest is the body for POST /platform/exports/verify.
type ExportVerificationRequest struct {
	VerificationSnap  *verifier.VerificationSnapshot  `json:"verification_snapshot,omitempty"`
	GovernanceSnap    *governance.GovernanceSnapshot   `json:"governance_snapshot,omitempty"`
	TopologySnap      *topology.TopologySnapshot       `json:"topology_snapshot,omitempty"`
	SchedulerSnap     *scheduler.SchedulingSnapshot    `json:"scheduler_snapshot,omitempty"`
}

// ─── Providers ─────────────────────────────────────────────────────────────

// ProviderCapabilityResponse is returned by GET /platform/providers.
type ProviderCapabilityResponse struct {
	Capabilities []topology.ProviderCapabilities `json:"capabilities"`
	Limitations  []string                        `json:"limitations"`
}
