package audit

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// SafetyViolationType classifies the kind of operator safety violation detected.
type SafetyViolationType string

const (
	ViolationUnsafeAutomation   SafetyViolationType = "unsafe_automation"
	ViolationPolicyBypass       SafetyViolationType = "policy_bypass"
	ViolationReplayCorruption   SafetyViolationType = "replay_corruption"
	ViolationQuarantineSuppression SafetyViolationType = "quarantine_suppression"
	ViolationUnauthorizedOverride SafetyViolationType = "unauthorized_override"
)

// SafetyViolation is an immutable record of a detected safety concern.
type SafetyViolation struct {
	ViolationType SafetyViolationType `json:"violation_type"`
	Severity      string              `json:"severity"` // critical, warning
	Description   string              `json:"description"`
	EntityRef     string              `json:"entity_ref,omitempty"`
}

// OperatorSafetyInput carries the signals needed to assess operator safety.
type OperatorSafetyInput struct {
	ExecutionID            string
	TenantID               string
	AutomatedActionsCount  int
	ApprovedActionsCount   int // must be >= AutomatedActionsCount for safety
	PolicyBypassAttempts   int
	ReplayDivergences      int
	QuarantinedCount       int
	QuarantineOverrides    int // operator-forced removal from quarantine without review
	UnauthorizedOperatorID string // non-empty if an unrecognized operatorID issued decisions
}

// OperatorSafetyAssessment is the result of a safety evaluation.
// AssessedAt is excluded from AssessmentHash (content-identity).
type OperatorSafetyAssessment struct {
	AssessmentID   string            `json:"assessment_id"`
	ExecutionID    string            `json:"execution_id"`
	TenantID       string            `json:"tenant_id"`
	Safe           bool              `json:"safe"`
	Violations     []SafetyViolation `json:"violations"`
	AssessedAt     string            `json:"assessed_at"` // excluded from hash
	AssessmentHash string            `json:"assessment_hash"`
}

// AssessOperatorSafety evaluates operator safety signals and returns a tamper-evident report.
func AssessOperatorSafety(assessmentID string, input OperatorSafetyInput) OperatorSafetyAssessment {
	a := OperatorSafetyAssessment{
		AssessmentID: assessmentID,
		ExecutionID:  input.ExecutionID,
		TenantID:     input.TenantID,
		AssessedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}

	if input.AutomatedActionsCount > 0 && input.ApprovedActionsCount < input.AutomatedActionsCount {
		a.Violations = append(a.Violations, SafetyViolation{
			ViolationType: ViolationUnsafeAutomation,
			Severity:      "critical",
			Description: fmt.Sprintf("%d automated action(s) executed without corresponding approval",
				input.AutomatedActionsCount-input.ApprovedActionsCount),
			EntityRef: input.ExecutionID,
		})
	}

	if input.PolicyBypassAttempts > 0 {
		a.Violations = append(a.Violations, SafetyViolation{
			ViolationType: ViolationPolicyBypass,
			Severity:      "critical",
			Description: fmt.Sprintf("%d policy bypass attempt(s) detected", input.PolicyBypassAttempts),
			EntityRef:     input.ExecutionID,
		})
	}

	if input.ReplayDivergences > 0 {
		a.Violations = append(a.Violations, SafetyViolation{
			ViolationType: ViolationReplayCorruption,
			Severity:      "critical",
			Description: fmt.Sprintf("%d replay divergence(s) — execution history may be corrupted", input.ReplayDivergences),
			EntityRef:     input.ExecutionID,
		})
	}

	if input.QuarantineOverrides > 0 {
		a.Violations = append(a.Violations, SafetyViolation{
			ViolationType: ViolationQuarantineSuppression,
			Severity:      "warning",
			Description: fmt.Sprintf("%d quarantine override(s) detected without review evidence", input.QuarantineOverrides),
			EntityRef:     input.ExecutionID,
		})
	}

	if input.UnauthorizedOperatorID != "" {
		a.Violations = append(a.Violations, SafetyViolation{
			ViolationType: ViolationUnauthorizedOverride,
			Severity:      "critical",
			Description: fmt.Sprintf("unrecognized operator %q issued decisions", input.UnauthorizedOperatorID),
			EntityRef:     input.UnauthorizedOperatorID,
		})
	}

	hasCritical := false
	for _, v := range a.Violations {
		if v.Severity == "critical" {
			hasCritical = true
			break
		}
	}
	a.Safe = !hasCritical
	a.AssessmentHash = computeSafetyHash(a)
	return a
}

// VerifySafetyHash recomputes the hash and returns (true, "") when it matches.
func VerifySafetyHash(a OperatorSafetyAssessment) (bool, string) {
	stored := a.AssessmentHash
	a.AssessmentHash = ""
	expected := computeSafetyHash(a)
	if stored != expected {
		return false, fmt.Sprintf("safety assessment %q hash mismatch: stored=%s computed=%s",
			a.AssessmentID, stored, expected)
	}
	return true, ""
}

func computeSafetyHash(a OperatorSafetyAssessment) string {
	h := sha256.New()
	fmt.Fprintf(h, "safety:%s|exec:%s|tenant:%s|safe:%v|violations:%d",
		a.AssessmentID, a.ExecutionID, a.TenantID, a.Safe, len(a.Violations))
	for _, v := range a.Violations {
		fmt.Fprintf(h, "|v:%s/%s", v.ViolationType, v.Severity)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
