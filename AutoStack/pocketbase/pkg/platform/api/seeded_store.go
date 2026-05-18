package api

import (
	"context"
	"fmt"
	"sync"

	"github.com/janlauber/one-click/pkg/governance"
	"github.com/janlauber/one-click/pkg/scheduler"
	"github.com/janlauber/one-click/pkg/topology"
	"github.com/janlauber/one-click/pkg/verifier"
)

// ─── seeded extras (consumed by timeline + forensics handlers) ────────────

// ContradictionSeed is one contradiction record with full evidence fields,
// suitable for surfacing on both the timeline and the forensics UI.
type ContradictionSeed struct {
	ContradictionID string                 `json:"contradiction_id"`
	Status          string                 `json:"status"` // unresolved | resolved | escalated
	Description     string                 `json:"description"`
	DetectedAt      string                 `json:"detected_at"`
	ResolvedAt      string                 `json:"resolved_at,omitempty"`
	ResolutionNote  string                 `json:"resolution_note,omitempty"`
	EvidenceFields  map[string]interface{} `json:"evidence_fields,omitempty"`
}

// AuditEntrySeed is one entry in the timeline's audit-chain view.
type AuditEntrySeed struct {
	EventType  string `json:"event_type"`
	Outcome    string `json:"outcome"`
	OccurredAt string `json:"occurred_at"`
}

// CertificationSeed is a snapshot of a per-execution operational certification.
type CertificationSeed struct {
	CertID      string
	Certified   bool
	GatesPassed int
	GatesTotal  int
	Gates       []SeededGate
	CertHash    string
	CertifiedAt string
}

// SeededGate mirrors ForensicGate without coupling the store to the response type.
type SeededGate struct {
	Name   string
	Passed bool
	Detail string
}

// BackupManifestSeed represents a per-execution backup manifest for the
// forensic backup-verification tab.
type BackupManifestSeed struct {
	BackupID      string
	Kind          string
	TenantID      string
	ExecutionID   string
	SchemaVersion string
	ManifestHash  string
	ContentHash   string
	CreatedAt     string
	ManifestValid bool
	PayloadValid  bool
}

// executionExtras bundles all forensic data for one execution.
type executionExtras struct {
	Phases         []timelinePhaseSeed
	Contradictions []ContradictionSeed
	AuditEntries   []AuditEntrySeed
	Certification  *CertificationSeed
	BackupManifest *BackupManifestSeed
	SnapshotHash   string
}

// timelinePhaseSeed is a richer execution-phase row for the timeline.
type timelinePhaseSeed struct {
	StageID   string
	Status    string
	StartedAt string
	EndedAt   string
	Note      string
}

// ─── SeededPlatformStore ───────────────────────────────────────────────────

// SeededPlatformStore is an in-process store pre-populated with synthetic
// demo executions. Three scenarios cover the platform's main operator states:
//   - exec-verified-1     — clean run, all stages green
//   - exec-contradicted-1 — provider observation conflict surfaced
//   - exec-pending-1      — paused on multi-approver governance gate
//
// All data is deterministic so demo screenshots are reproducible.
type SeededPlatformStore struct {
	mu sync.RWMutex
	*InMemoryPlatformStore

	extras map[string]*executionExtras
}

// ExtrasFor returns the seeded forensic/timeline extras for an execution, or
// nil if the execution is unknown.
func (s *SeededPlatformStore) ExtrasFor(executionID string) *executionExtras {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.extras[executionID]
}

// NewSeededPlatformStore returns a store seeded with three example executions.
func NewSeededPlatformStore() *SeededPlatformStore {
	base := NewInMemoryPlatformStore()
	base.Summaries = []ExecutionSummary{
		{
			ExecutionID:       "exec-verified-1",
			VerificationState: string(verifier.VerificationVerified),
			ConfidenceScore:   0.98,
			TotalStages:       5,
			CompletedStages:   5,
			AuditEntryCount:   42,
			HasContradictions: false,
			HasOverrides:      false,
		},
		{
			ExecutionID:       "exec-contradicted-1",
			VerificationState: string(verifier.VerificationContradicted),
			ConfidenceScore:   0.62,
			TotalStages:       7,
			CompletedStages:   4,
			AuditEntryCount:   89,
			HasContradictions: true,
			HasOverrides:      false,
		},
		{
			ExecutionID:       "exec-pending-1",
			VerificationState: "pending",
			ConfidenceScore:   0.0,
			TotalStages:       3,
			CompletedStages:   1,
			AuditEntryCount:   12,
			HasContradictions: false,
			HasOverrides:      true,
		},
	}

	for _, s := range base.Summaries {
		base.Details[s.ExecutionID] = &ExecutionDetailResponse{
			ExecutionID: s.ExecutionID,
			Limitations: []string{
				"Seeded demo data; not produced by a live execution engine.",
			},
		}
		base.Topologies[s.ExecutionID] = &topology.TopologySnapshot{ExecutionID: s.ExecutionID}
	}

	// Per-execution governance, replay, and extras.
	base.Governances["exec-verified-1"] = seededGovVerified()
	base.Governances["exec-contradicted-1"] = seededGovContradicted()
	base.Governances["exec-pending-1"] = seededGovPending()

	base.ReplayEvents["exec-verified-1"], base.ReplayOrders["exec-verified-1"] = seededReplayVerified()
	base.ReplayEvents["exec-contradicted-1"], base.ReplayOrders["exec-contradicted-1"] = seededReplayContradicted()
	base.ReplayEvents["exec-pending-1"], base.ReplayOrders["exec-pending-1"] = seededReplayPending()

	extras := map[string]*executionExtras{
		"exec-verified-1":     seededExtrasVerified(),
		"exec-contradicted-1": seededExtrasContradicted(),
		"exec-pending-1":      seededExtrasPending(),
	}

	return &SeededPlatformStore{InMemoryPlatformStore: base, extras: extras}
}

func (s *SeededPlatformStore) ListExecutionSummaries(ctx context.Context) ([]ExecutionSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.InMemoryPlatformStore.ListExecutionSummaries(ctx)
}

func (s *SeededPlatformStore) GetExecutionDetail(ctx context.Context, id string) (*ExecutionDetailResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, err := s.InMemoryPlatformStore.GetExecutionDetail(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("execution %q not found in seeded store", id)
	}
	return d, nil
}

// ─── per-scenario seed builders ────────────────────────────────────────────

func seededGovVerified() *governance.GovernanceSnapshot {
	return &governance.GovernanceSnapshot{
		ExecutionID: "exec-verified-1",
		Decisions: []governance.PolicyDecision{
			{RequestID: "req-v1-1", PolicyID: "policy-deploy-allow", Effect: governance.PolicyEffectAllow,
				Reason: "deployment within tenant quota; no policy violations", DecidedAt: "2026-05-17T03:00:00Z"},
			{RequestID: "req-v1-2", PolicyID: "policy-health-probe", Effect: governance.PolicyEffectAllow,
				Reason: "post-deploy health probe within tolerance", DecidedAt: "2026-05-17T03:04:30Z"},
		},
	}
}

func seededGovContradicted() *governance.GovernanceSnapshot {
	return &governance.GovernanceSnapshot{
		ExecutionID: "exec-contradicted-1",
		Decisions: []governance.PolicyDecision{
			{RequestID: "req-c1-1", PolicyID: "policy-deploy-allow", Effect: governance.PolicyEffectAllow,
				Reason: "deployment authorized", DecidedAt: "2026-05-17T02:45:00Z"},
			{RequestID: "req-c1-2", PolicyID: "policy-rollback-gate", Effect: governance.PolicyEffectRequireApproval,
				Reason: "contradiction detected; rollback requires operator confirmation", DecidedAt: "2026-05-17T03:12:00Z"},
		},
	}
}

func seededGovPending() *governance.GovernanceSnapshot {
	return &governance.GovernanceSnapshot{
		ExecutionID: "exec-pending-1",
		Decisions: []governance.PolicyDecision{
			{RequestID: "req-p1-1", PolicyID: "policy-multi-approval", Effect: governance.PolicyEffectRequireMultiApproval,
				Reason: "production-tier deploy requires two approvers; one pending", DecidedAt: "2026-05-17T03:30:00Z"},
		},
	}
}

func seededReplayVerified() ([]scheduler.SchedulerReplayEvent, []string) {
	return []scheduler.SchedulerReplayEvent{
		{Sequence: 1, EventType: "enqueued", StageID: "plan", Timestamp: "2026-05-17T03:00:01Z"},
		{Sequence: 2, EventType: "dequeued", StageID: "plan", Timestamp: "2026-05-17T03:00:02Z"},
		{Sequence: 3, EventType: "enqueued", StageID: "deploy", Timestamp: "2026-05-17T03:00:10Z"},
		{Sequence: 4, EventType: "dequeued", StageID: "deploy", Timestamp: "2026-05-17T03:00:15Z"},
		{Sequence: 5, EventType: "enqueued", StageID: "verify", Timestamp: "2026-05-17T03:00:30Z"},
		{Sequence: 6, EventType: "dequeued", StageID: "verify", Timestamp: "2026-05-17T03:00:35Z"},
	}, []string{"plan", "deploy", "verify"}
}

func seededReplayContradicted() ([]scheduler.SchedulerReplayEvent, []string) {
	return []scheduler.SchedulerReplayEvent{
		{Sequence: 1, EventType: "enqueued", StageID: "plan", Timestamp: "2026-05-17T02:45:01Z"},
		{Sequence: 2, EventType: "dequeued", StageID: "plan", Timestamp: "2026-05-17T02:45:02Z"},
		{Sequence: 3, EventType: "enqueued", StageID: "deploy-canary", Timestamp: "2026-05-17T02:50:00Z"},
		{Sequence: 4, EventType: "dequeued", StageID: "deploy-canary", Timestamp: "2026-05-17T02:50:05Z"},
		{Sequence: 5, EventType: "enqueued", StageID: "deploy-prod", Timestamp: "2026-05-17T03:00:00Z"},
		{Sequence: 6, EventType: "dequeued", StageID: "deploy-prod", Timestamp: "2026-05-17T03:00:30Z"},
		{Sequence: 7, EventType: "blocked", StageID: "verify-prod", Timestamp: "2026-05-17T03:12:00Z"},
	}, []string{"plan", "deploy-canary", "deploy-prod", "verify-prod"}
}

func seededReplayPending() ([]scheduler.SchedulerReplayEvent, []string) {
	return []scheduler.SchedulerReplayEvent{
		{Sequence: 1, EventType: "enqueued", StageID: "plan", Timestamp: "2026-05-17T03:30:01Z"},
		{Sequence: 2, EventType: "dequeued", StageID: "plan", Timestamp: "2026-05-17T03:30:02Z"},
		{Sequence: 3, EventType: "paused", StageID: "deploy", Timestamp: "2026-05-17T03:30:05Z",
			Metadata: map[string]string{"reason": "awaiting multi-approval"}},
	}, []string{"plan", "deploy"}
}

func seededExtrasVerified() *executionExtras {
	return &executionExtras{
		Phases: []timelinePhaseSeed{
			{StageID: "plan", Status: "completed", StartedAt: "2026-05-17T03:00:01Z", EndedAt: "2026-05-17T03:00:02Z", Note: "deterministic plan resolved"},
			{StageID: "deploy", Status: "completed", StartedAt: "2026-05-17T03:00:10Z", EndedAt: "2026-05-17T03:00:14Z", Note: "rolling deploy successful"},
			{StageID: "verify", Status: "completed", StartedAt: "2026-05-17T03:00:30Z", EndedAt: "2026-05-17T03:00:34Z", Note: "post-deploy health probe passed"},
		},
		AuditEntries: []AuditEntrySeed{
			{EventType: "auth.success", Outcome: "allowed", OccurredAt: "2026-05-17T03:00:00Z"},
			{EventType: "governance.decision", Outcome: "allow", OccurredAt: "2026-05-17T03:00:01Z"},
			{EventType: "execution.completed", Outcome: "verified", OccurredAt: "2026-05-17T03:00:35Z"},
		},
		Certification: &CertificationSeed{
			CertID: "cert-verified-1", Certified: true, GatesPassed: 9, GatesTotal: 9,
			CertifiedAt: "2026-05-17T03:00:40Z", CertHash: "sha256:b1a4f8d2c1e9a7b6c3d2e1f0a9b8c7d6",
			Gates: []SeededGate{
				{Name: "replay-continuity", Passed: true, Detail: "manifest hash stable across restart"},
				{Name: "audit-durability", Passed: true, Detail: "chain intact; 42 entries verified"},
				{Name: "governance-continuity", Passed: true, Detail: "2 decisions preserved"},
				{Name: "backup-recoverability", Passed: true, Detail: "manifest + payload verified"},
				{Name: "contradiction-survivability", Passed: true, Detail: "scan complete; 0 unresolved"},
				{Name: "restart-survivability", Passed: true, Detail: "store recoverable"},
				{Name: "revocation-integrity", Passed: true, Detail: "0 revocations"},
				{Name: "deterministic-exports", Passed: true, Detail: "export hashes stable"},
				{Name: "staging-deployment", Passed: true, Detail: "/healthz responded 200"},
			},
		},
		BackupManifest: &BackupManifestSeed{
			BackupID: "bk-verified-1-2026-05-17", Kind: "platform-snapshot", TenantID: "tenant-a",
			ExecutionID: "exec-verified-1", SchemaVersion: "7.0.0",
			ManifestHash: "sha256:e7d4a3b1c2f5d6e7a8b9c0d1e2f3a4b5",
			ContentHash:  "sha256:a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
			CreatedAt:    "2026-05-17T03:01:00Z", ManifestValid: true, PayloadValid: true,
		},
		SnapshotHash: "sha256:c9d1e8f3a4b5c6d7e8f9a0b1c2d3e4f5",
	}
}

func seededExtrasContradicted() *executionExtras {
	return &executionExtras{
		Phases: []timelinePhaseSeed{
			{StageID: "plan", Status: "completed", StartedAt: "2026-05-17T02:45:01Z", EndedAt: "2026-05-17T02:45:02Z", Note: "plan resolved"},
			{StageID: "deploy-canary", Status: "completed", StartedAt: "2026-05-17T02:50:00Z", EndedAt: "2026-05-17T02:50:04Z", Note: "canary 5% traffic shifted"},
			{StageID: "deploy-prod", Status: "completed", StartedAt: "2026-05-17T03:00:00Z", EndedAt: "2026-05-17T03:00:28Z", Note: "prod rollout complete"},
			{StageID: "verify-prod", Status: "blocked", StartedAt: "2026-05-17T03:12:00Z", Note: "provider observation contradicts desired state"},
		},
		Contradictions: []ContradictionSeed{
			{
				ContradictionID: "con-prod-replicaset-mismatch",
				Status:          "unresolved",
				Description:     "AWS ECS reports 4 running tasks but the desired count is 6 — provider observation contradicts platform state",
				DetectedAt:      "2026-05-17T03:12:00Z",
				EvidenceFields: map[string]interface{}{
					"desired_count":           6,
					"observed_running_count":  4,
					"provider":                "aws-ecs",
					"cluster_arn":             "arn:aws:ecs:us-east-1:000000:cluster/prod",
					"last_provider_event":     "TaskStopped (CapacityProviderError)",
					"last_observation_window": "2026-05-17T03:10:00Z..2026-05-17T03:12:00Z",
				},
			},
		},
		AuditEntries: []AuditEntrySeed{
			{EventType: "auth.success", Outcome: "allowed", OccurredAt: "2026-05-17T02:45:00Z"},
			{EventType: "governance.decision", Outcome: "allow", OccurredAt: "2026-05-17T02:45:01Z"},
			{EventType: "contradiction.detected", Outcome: "surfaced-to-operator", OccurredAt: "2026-05-17T03:12:00Z"},
			{EventType: "governance.decision", Outcome: "require-approval", OccurredAt: "2026-05-17T03:12:01Z"},
		},
		Certification: &CertificationSeed{
			CertID: "cert-contradicted-1", Certified: false, GatesPassed: 8, GatesTotal: 9,
			CertifiedAt: "2026-05-17T03:13:00Z", CertHash: "sha256:f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5",
			Gates: []SeededGate{
				{Name: "replay-continuity", Passed: true, Detail: "manifest hash stable"},
				{Name: "audit-durability", Passed: true, Detail: "chain intact; 89 entries"},
				{Name: "governance-continuity", Passed: true, Detail: "2 decisions preserved"},
				{Name: "backup-recoverability", Passed: true, Detail: "manifest + payload verified"},
				{Name: "contradiction-survivability", Passed: false, Detail: "1 unresolved contradiction visible to operators"},
				{Name: "restart-survivability", Passed: true, Detail: "store recoverable"},
				{Name: "revocation-integrity", Passed: true, Detail: "0 revocations"},
				{Name: "deterministic-exports", Passed: true, Detail: "export hashes stable"},
				{Name: "staging-deployment", Passed: true, Detail: "/healthz responded 200"},
			},
		},
		BackupManifest: &BackupManifestSeed{
			BackupID: "bk-contradicted-1-2026-05-17", Kind: "platform-snapshot", TenantID: "tenant-a",
			ExecutionID: "exec-contradicted-1", SchemaVersion: "7.0.0",
			ManifestHash: "sha256:d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3",
			ContentHash:  "sha256:b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9",
			CreatedAt:    "2026-05-17T03:01:00Z", ManifestValid: true, PayloadValid: true,
		},
		SnapshotHash: "sha256:a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2",
	}
}

func seededExtrasPending() *executionExtras {
	return &executionExtras{
		Phases: []timelinePhaseSeed{
			{StageID: "plan", Status: "completed", StartedAt: "2026-05-17T03:30:01Z", EndedAt: "2026-05-17T03:30:02Z", Note: "plan resolved"},
			{StageID: "deploy", Status: "blocked", StartedAt: "2026-05-17T03:30:05Z", Note: "awaiting second approver (multi-approval policy)"},
		},
		AuditEntries: []AuditEntrySeed{
			{EventType: "auth.success", Outcome: "allowed", OccurredAt: "2026-05-17T03:30:00Z"},
			{EventType: "governance.decision", Outcome: "require-multi-approval", OccurredAt: "2026-05-17T03:30:01Z"},
			{EventType: "approval.received", Outcome: "1-of-2", OccurredAt: "2026-05-17T03:32:00Z"},
		},
		Certification: nil,
		BackupManifest: nil,
		SnapshotHash:   "sha256:e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
	}
}
