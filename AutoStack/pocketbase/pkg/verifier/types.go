// Package verifier provides the execution verification layer that distinguishes
// provider acceptance from verified operational convergence.
//
// Core principle:
//   provider accepted request ≠ operation succeeded
//   operation succeeded ≠ state verified
//   state verified ≠ state stable
//
// The verifier models the full lifecycle of provider truth:
//
//	submitted → acknowledged → stabilizing → partially_verified → verified
//	                                       ↘ contradicted
//	                                       ↘ stale
//	                                       ↘ unverifiable
//
// Every state is explicitly persisted. Ambiguity is surfaced, never flattened.
// No automatic recovery from any non-verified state.
package verifier

import (
	"github.com/janlauber/one-click/pkg/planner"
)

// VerificationState describes where a node operation stands in the convergence lifecycle.
// All states are persisted before being applied — replay reconstructs the full timeline.
type VerificationState string

const (
	// VerificationSubmitted: operation request sent to provider, no acknowledgement yet.
	VerificationSubmitted VerificationState = "submitted"
	// VerificationAcknowledged: provider confirmed it received the request.
	// Does NOT mean the operation started or will succeed.
	VerificationAcknowledged VerificationState = "acknowledged"
	// VerificationStabilizing: observations started, waiting for stabilization window to close.
	VerificationStabilizing VerificationState = "stabilizing"
	// VerificationPartiallyVerified: stabilization conditions partially met, confidence below threshold.
	VerificationPartiallyVerified VerificationState = "partially_verified"
	// VerificationVerified: all convergence conditions met, confidence above threshold, no contradictions.
	VerificationVerified VerificationState = "verified"
	// VerificationContradicted: two or more observations report incompatible states.
	// Operator review required before proceeding.
	VerificationContradicted VerificationState = "contradicted"
	// VerificationStale: all available observations are older than the policy's MaxStaleness.
	// Re-querying the provider is required.
	VerificationStale VerificationState = "stale"
	// VerificationUnverifiable: cannot determine the true provider state.
	// Sources: no observations, ambiguous provider response, capability gap.
	VerificationUnverifiable VerificationState = "unverifiable"
)

// TrustLevel classifies how much confidence to place in a single provider observation.
// Maps to the planner.ConfidenceLevel taxonomy used in Phase 3.4+.
type TrustLevel string

const (
	// TrustHigh: provider has a definitive, unambiguous signal.
	TrustHigh TrustLevel = "high"
	// TrustMedium: provider has a partial or intermediate signal.
	TrustMedium TrustLevel = "medium"
	// TrustLow: provider gave ambiguous, conflicting, or incomplete signal.
	TrustLow TrustLevel = "low"
)

// ProviderObservation is one timestamped provider status reading.
// Observations form an ordered timeline from which convergence is derived.
// They are immutable once recorded.
type ProviderObservation struct {
	// ObservationID is a stable, unique identifier for replay.
	ObservationID string `json:"observation_id"`
	// ExecutionID ties this observation to an execution.
	ExecutionID string `json:"execution_id"`
	// StageID ties this observation to a stage.
	StageID string `json:"stage_id"`
	// NodeID ties this observation to a specific node.
	NodeID string `json:"node_id"`
	// ObservedAt is the UTC RFC3339 time this observation was recorded.
	ObservedAt string `json:"observed_at"`
	// ProviderState is the normalized state string (lowercase): "running", "failed", etc.
	ProviderState string `json:"provider_state"`
	// NativeState is the verbatim provider-native state, preserved for forensics.
	// Cloud Run: "SERVING"|"FAILED"|"UPDATING". ECS: "RUNNING"|"STOPPED".
	NativeState string `json:"native_state"`
	// TrustLevel classifies the reliability of this observation.
	TrustLevel TrustLevel `json:"trust_level"`
	// VerificationSource describes how this observation was obtained.
	// "get_status" | "get_operation" | "polling" | "event"
	VerificationSource string `json:"verification_source"`
	// StaleFlag is set true when this observation exceeds the policy MaxStaleness.
	StaleFlag bool `json:"stale_flag"`
	// ContradictionFlag is set true when this observation contradicts a prior observation.
	ContradictionFlag bool `json:"contradiction_flag"`
	// ConfidenceContribution is this observation's weight in confidence scoring.
	ConfidenceContribution float64 `json:"confidence_contribution"`
}

// ObservationTimeline is an ordered sequence of provider observations for one node.
// Observations are sorted by ObservedAt ascending — the last entry is the most recent.
// The timeline is append-only: no observation is ever removed or modified.
type ObservationTimeline struct {
	ExecutionID  string                `json:"execution_id"`
	StageID      string                `json:"stage_id"`
	NodeID       string                `json:"node_id"`
	Observations []ProviderObservation `json:"observations"`
	CreatedAt    string                `json:"created_at"`
}

// StabilizationWindow defines the minimum conditions for declaring convergence.
// Every provider has different stabilization semantics; use the provider-specific
// window rather than a global constant.
//
// Convergence is NOT declared until ALL three conditions are satisfied:
//  1. WindowDuration has elapsed since the operation was submitted.
//  2. At least RequiredSamples observations exist.
//  3. The most recent ConsistencyPeriod of observations all report the same ProviderState.
type StabilizationWindow struct {
	// Provider identifies which cloud provider this window applies to.
	Provider string `json:"provider"`
	// WindowDuration is the minimum elapsed time since operation submission.
	// Even if the provider reports "running" immediately, we wait this long.
	WindowDuration int64 `json:"window_duration_ns"` // nanoseconds for JSON safety
	// RequiredSamples is the minimum number of observations before convergence is possible.
	RequiredSamples int `json:"required_samples"`
	// ConsistencyPeriod is the trailing window in which all observations must agree.
	ConsistencyPeriod int64 `json:"consistency_period_ns"` // nanoseconds
	// Description is an operator-readable explanation of this window's rationale.
	Description string `json:"description"`
	// Limitations documents known gaps in this window's guarantees.
	Limitations []string `json:"limitations"`
}

// ConsistencyPolicy describes provider-specific eventual consistency behavior.
// The verifier uses the policy to decide when observations are stale and how
// to interpret intermediate states.
type ConsistencyPolicy struct {
	// Provider identifies which cloud provider this policy applies to.
	Provider string `json:"provider"`
	// EventualConsistency is true when the provider does not guarantee immediate consistency.
	EventualConsistency bool `json:"eventual_consistency"`
	// MaxStaleness is the maximum age of an observation before it is considered stale.
	MaxStaleness int64 `json:"max_staleness_ns"` // nanoseconds
	// PropagationDelay is the expected delay before a state change is visible.
	PropagationDelay int64 `json:"propagation_delay_ns"` // nanoseconds
	// RequiresPolling is true when the provider has no push notification capability.
	RequiresPolling bool `json:"requires_polling"`
	// Description is an operator-readable summary of this policy.
	Description string `json:"description"`
}

// Contradiction is a recorded inconsistency between two observations in a timeline.
// A contradiction does NOT automatically fail the execution — it requires operator review.
type Contradiction struct {
	// ContradictionID is a stable unique identifier for replay.
	ContradictionID string `json:"contradiction_id"`
	// ObservationA is the earlier observation.
	ObservationA ProviderObservation `json:"observation_a"`
	// ObservationB is the later observation that contradicts ObservationA.
	ObservationB ProviderObservation `json:"observation_b"`
	// Reason is an operator-readable description of why these observations contradict.
	Reason string `json:"reason"`
	// DetectedAt is when this contradiction was identified.
	DetectedAt string `json:"detected_at"`
}

// ConfidenceScore captures three orthogonal confidence dimensions.
// All values are in [0.0, 1.0]. A value of 1.0 means maximum confidence.
// Confidence degrades under staleness, contradictions, and low-trust observations.
type ConfidenceScore struct {
	// ExecutionConfidence: confidence that the execution as a whole is progressing correctly.
	ExecutionConfidence float64 `json:"execution_confidence"`
	// ProviderConfidence: confidence in the accuracy of provider-reported state.
	ProviderConfidence float64 `json:"provider_confidence"`
	// VerificationConfidence: confidence that verification conclusions are accurate.
	VerificationConfidence float64 `json:"verification_confidence"`
	// DegradationReasons lists why confidence was reduced from 1.0.
	DegradationReasons []string `json:"degradation_reasons,omitempty"`
}

// VerificationContext is the input to VerifyExecutionConsistency.
// It is deterministic: same inputs always produce the same result.
type VerificationContext struct {
	// ExecutionID ties verification to an execution run.
	ExecutionID string
	// StageID identifies the execution stage.
	StageID string
	// NodeID identifies the specific node being verified.
	NodeID string
	// Provider identifies the cloud provider implementation.
	Provider string
	// Action is the intended operation being verified.
	Action planner.ActionType
	// Timeline is the ordered sequence of provider observations.
	Timeline ObservationTimeline
	// StabilizationWindow is the provider-specific convergence window.
	StabilizationWindow StabilizationWindow
	// Policy is the provider-specific consistency policy.
	Policy ConsistencyPolicy
	// SubmittedAt is the UTC RFC3339 time the operation was submitted to the provider.
	SubmittedAt string
	// RefTime is the reference time for staleness calculations (typically time.Now()).
	// Explicit injection makes the engine deterministic and test-friendly.
	RefTime string
}

// VerificationResult is the output of VerifyExecutionConsistency.
// It is deterministic: same VerificationContext always produces the same result.
type VerificationResult struct {
	// ExecutionID ties the result to an execution.
	ExecutionID string `json:"execution_id"`
	// StageID identifies the stage.
	StageID string `json:"stage_id"`
	// NodeID identifies the node.
	NodeID string `json:"node_id"`
	// State is the current verification lifecycle state.
	State VerificationState `json:"state"`
	// Contradictions lists all detected observation contradictions.
	Contradictions []Contradiction `json:"contradictions,omitempty"`
	// Confidence is the three-dimensional confidence score.
	Confidence ConfidenceScore `json:"confidence"`
	// StabilizationMet reports whether the stabilization window conditions were satisfied.
	StabilizationMet bool `json:"stabilization_met"`
	// Evidence is a list of operator-readable evidence strings supporting the verdict.
	Evidence []string `json:"evidence,omitempty"`
	// Recommendation is an operator-readable action recommendation.
	Recommendation string `json:"recommendation"`
	// Limitations documents known gaps in this verification result.
	Limitations []string `json:"limitations"`
	// VerifiedAt is set when State == VerificationVerified.
	VerifiedAt string `json:"verified_at,omitempty"`
}

// VerificationSnapshot is a tamper-evident export of a VerificationResult.
// GeneratedAt is excluded from ExportHash so time-of-export does not invalidate the hash.
type VerificationSnapshot struct {
	SchemaVersion       string               `json:"schema_version"`
	ExecutionID         string               `json:"execution_id"`
	StageID             string               `json:"stage_id"`
	NodeID              string               `json:"node_id"`
	State               VerificationState    `json:"state"`
	ObservationCount    int                  `json:"observation_count"`
	ContradictionCount  int                  `json:"contradiction_count"`
	Confidence          ConfidenceScore      `json:"confidence"`
	StabilizationMet    bool                 `json:"stabilization_met"`
	StabilizationWindow StabilizationWindow  `json:"stabilization_window"`
	Policy              ConsistencyPolicy    `json:"policy"`
	Evidence            []string             `json:"evidence,omitempty"`
	Contradictions      []Contradiction      `json:"contradictions,omitempty"`
	Recommendation      string               `json:"recommendation"`
	Limitations         []string             `json:"limitations"`
	VerifiedAt          string               `json:"verified_at,omitempty"`
	GeneratedAt         string               `json:"generated_at"`
	ExportHash          string               `json:"export_hash"`
}
