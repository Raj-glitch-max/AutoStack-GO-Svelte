package providerops

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// ContradictionType classifies the specific contradiction pattern detected.
type ContradictionType string

const (
	// ContradictionDeploySuccessResourceMissing: deploy reported success but resource is absent.
	ContradictionDeploySuccessResourceMissing ContradictionType = "deploy_success_resource_missing"
	// ContradictionDeleteSuccessResourceAlive: delete reported success but resource still exists.
	ContradictionDeleteSuccessResourceAlive ContradictionType = "delete_success_resource_alive"
	// ContradictionRollbackSuccessUnhealthy: rollback reported success but resource is unhealthy.
	ContradictionRollbackSuccessUnhealthy ContradictionType = "rollback_success_unhealthy"
	// ContradictionScaleAcknowledgedCapacityUnchanged: scale acknowledged but capacity did not change.
	ContradictionScaleAcknowledgedCapacityUnchanged ContradictionType = "scale_acknowledged_capacity_unchanged"
)

// ProviderContradiction is an immutable, hashed contradiction record.
// DetectedAt is excluded from the hash (content-identity).
type ProviderContradiction struct {
	ContradictionID  string            `json:"contradiction_id"`
	OperationID      string            `json:"operation_id"`
	ExecutionID      string            `json:"execution_id"`
	TenantID         string            `json:"tenant_id"`
	ProviderID       string            `json:"provider_id"`
	Type             ContradictionType `json:"type"`
	OperationType    OperationType     `json:"operation_type"`
	ReportedResult   ResultState       `json:"reported_result"`
	ObservedState    string            `json:"observed_state"`
	ConfidenceImpact float64           `json:"confidence_impact"`
	Resolved         bool              `json:"resolved"`
	DetectedAt       string            `json:"detected_at"` // excluded from hash
	ContradictionHash string           `json:"contradiction_hash"`
}

// ContradictionInput carries the signals needed for contradiction detection.
type ContradictionInput struct {
	OperationID    string        `json:"operation_id"`
	OperationType  OperationType `json:"operation_type"`
	ReportedResult ResultState   `json:"reported_result"`
	ResourceExists bool          `json:"resource_exists"`
	ResourceHealthy bool         `json:"resource_healthy"`
	ObservedCapacity int         `json:"observed_capacity"`
	RequestedCapacity int        `json:"requested_capacity"`
}

// DetectProviderContradiction examines provider signals and returns a contradiction
// record when one is detected. Returns (nil, nil) when the signals are consistent.
func DetectProviderContradiction(contradictionID, executionID, tenantID, providerID string, input ContradictionInput) (*ProviderContradiction, error) {
	ctype, observed, impact := detectContradictionType(input)
	if ctype == "" {
		return nil, nil
	}

	c := ProviderContradiction{
		ContradictionID:  contradictionID,
		OperationID:      input.OperationID,
		ExecutionID:      executionID,
		TenantID:         tenantID,
		ProviderID:       providerID,
		Type:             ctype,
		OperationType:    input.OperationType,
		ReportedResult:   input.ReportedResult,
		ObservedState:    observed,
		ConfidenceImpact: impact,
		DetectedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	c.ContradictionHash = computeContradictionHash(c)
	return &c, nil
}

// VerifyContradictionHash recomputes the hash and returns (true, "") when it matches.
func VerifyContradictionHash(c ProviderContradiction) (bool, string) {
	stored := c.ContradictionHash
	c.ContradictionHash = ""
	expected := computeContradictionHash(c)
	if stored != expected {
		return false, fmt.Sprintf("contradiction %q hash mismatch: stored=%s computed=%s",
			c.ContradictionID, stored, expected)
	}
	return true, ""
}

func detectContradictionType(input ContradictionInput) (ContradictionType, string, float64) {
	switch input.OperationType {
	case OperationTypeDeploy:
		if input.ReportedResult == ResultStateSuccess && !input.ResourceExists {
			return ContradictionDeploySuccessResourceMissing, "resource_absent", 0.40
		}
	case OperationTypeDestroy:
		if input.ReportedResult == ResultStateSuccess && input.ResourceExists {
			return ContradictionDeleteSuccessResourceAlive, "resource_alive", 0.35
		}
	case OperationTypeRollback:
		if input.ReportedResult == ResultStateSuccess && !input.ResourceHealthy {
			return ContradictionRollbackSuccessUnhealthy, "resource_unhealthy", 0.30
		}
	case OperationTypeScale:
		if input.ReportedResult == ResultStateSuccess &&
			input.RequestedCapacity > 0 &&
			input.ObservedCapacity == 0 {
			return ContradictionScaleAcknowledgedCapacityUnchanged, "capacity_unchanged", 0.25
		}
	}
	return "", "", 0
}

func computeContradictionHash(c ProviderContradiction) string {
	h := sha256.New()
	fmt.Fprintf(h, "contradiction:%s|op:%s|exec:%s|tenant:%s|provider:%s|type:%s|optype:%s|result:%s|observed:%s|impact:%.4f|resolved:%v",
		c.ContradictionID, c.OperationID, c.ExecutionID, c.TenantID, c.ProviderID,
		c.Type, c.OperationType, c.ReportedResult, c.ObservedState,
		c.ConfidenceImpact, c.Resolved)
	return fmt.Sprintf("%x", h.Sum(nil))
}
