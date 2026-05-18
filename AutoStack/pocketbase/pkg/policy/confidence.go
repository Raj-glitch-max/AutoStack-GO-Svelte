package policy

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ConfidenceLevel is a categorical label derived from the composite score.
type ConfidenceLevel string

const (
	ConfidenceLevelHigh     ConfidenceLevel = "high"     // score >= 0.80
	ConfidenceLevelMedium   ConfidenceLevel = "medium"   // score >= 0.50
	ConfidenceLevelLow      ConfidenceLevel = "low"      // score >= 0.20
	ConfidenceLevelCritical ConfidenceLevel = "critical" // score < 0.20
)

// RuntimeConfidenceAssessment is a tamper-evident, deterministic confidence score
// computed from multiple operational signals.
//
// Formula (deterministic — no ML, no probabilistic rounding):
//
//	base  = verificationConf*0.30 + providerConf*0.30 + workflowStability*0.40
//	penalty = contradictions*0.10 + replayDivergences*0.15
//	score = clamp(base - penalty, 0.0, 1.0)
type RuntimeConfidenceAssessment struct {
	AssessmentID       string          `json:"assessment_id"`
	ExecutionID        string          `json:"execution_id"`
	TenantID           string          `json:"tenant_id"`
	VerificationConf   float64         `json:"verification_conf"`    // 0.0–1.0
	ProviderConf       float64         `json:"provider_conf"`        // 0.0–1.0
	WorkflowStability  float64         `json:"workflow_stability"`   // 0.0–1.0
	ContradictionCount int             `json:"contradiction_count"`
	ReplayDivergences  int             `json:"replay_divergences"`
	ApprovalStatus     string          `json:"approval_status"` // informational
	CompositeScore     float64         `json:"composite_score"`
	Level              ConfidenceLevel `json:"level"`
	AssessedAt         string          `json:"assessed_at"` // excluded from hash
	AssessmentHash     string          `json:"assessment_hash"`
}

// ComputeConfidence derives a deterministic confidence assessment from raw signals.
// All inputs are validated to [0.0, 1.0] before use.
func ComputeConfidence(
	assessmentID, executionID, tenantID string,
	verificationConf, providerConf, workflowStability float64,
	contradictionCount, replayDivergences int,
	approvalStatus string,
) RuntimeConfidenceAssessment {
	// Clamp inputs.
	verificationConf = clamp(verificationConf)
	providerConf = clamp(providerConf)
	workflowStability = clamp(workflowStability)

	base := verificationConf*0.30 + providerConf*0.30 + workflowStability*0.40
	penalty := float64(contradictionCount)*0.10 + float64(replayDivergences)*0.15
	score := clamp(base - penalty)

	a := RuntimeConfidenceAssessment{
		AssessmentID:       assessmentID,
		ExecutionID:        executionID,
		TenantID:           tenantID,
		VerificationConf:   verificationConf,
		ProviderConf:       providerConf,
		WorkflowStability:  workflowStability,
		ContradictionCount: contradictionCount,
		ReplayDivergences:  replayDivergences,
		ApprovalStatus:     approvalStatus,
		CompositeScore:     score,
		Level:              ConfidenceLevelFromScore(score),
		AssessedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	a.AssessmentHash = computeAssessmentHash(a)
	return a
}

// ConfidenceLevelFromScore returns the categorical level for a given score.
func ConfidenceLevelFromScore(score float64) ConfidenceLevel {
	switch {
	case score >= 0.80:
		return ConfidenceLevelHigh
	case score >= 0.50:
		return ConfidenceLevelMedium
	case score >= 0.20:
		return ConfidenceLevelLow
	default:
		return ConfidenceLevelCritical
	}
}

// VerifyAssessmentHash recomputes the hash and returns (true, "") when it matches.
// AssessedAt is excluded from the hash (content-identity).
func VerifyAssessmentHash(a RuntimeConfidenceAssessment) (bool, string) {
	stored := a.AssessmentHash
	a.AssessmentHash = ""
	expected := computeAssessmentHash(a)
	if stored != expected {
		return false, fmt.Sprintf("assessment %q hash mismatch: stored=%s computed=%s",
			a.AssessmentID, stored, expected)
	}
	return true, ""
}

// BelowOperatingThreshold returns true when the score is at critical level.
// At critical level, autonomous operations should be halted pending operator review.
func BelowOperatingThreshold(a RuntimeConfidenceAssessment) bool {
	return a.Level == ConfidenceLevelCritical
}

func clamp(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

func computeAssessmentHash(a RuntimeConfidenceAssessment) string {
	h := sha256.New()
	fmt.Fprintf(h,
		"assess:%s|exec:%s|tenant:%s|vconf:%.6f|pconf:%.6f|wstab:%.6f|contradictions:%d|divergences:%d|approval:%s|score:%.6f|level:%s",
		a.AssessmentID, a.ExecutionID, a.TenantID,
		a.VerificationConf, a.ProviderConf, a.WorkflowStability,
		a.ContradictionCount, a.ReplayDivergences, a.ApprovalStatus,
		a.CompositeScore, a.Level,
	)
	return fmt.Sprintf("%x", h.Sum(nil))
}
