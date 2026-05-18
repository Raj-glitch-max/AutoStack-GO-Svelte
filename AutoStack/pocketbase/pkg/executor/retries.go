package executor

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// RetryPolicy defines the retry behaviour for a task.
type RetryPolicy struct {
	MaxAttempts    int `json:"max_attempts"`
	BackoffBaseSec int `json:"backoff_base_sec"` // seconds for attempt 1
	BackoffMaxSec  int `json:"backoff_max_sec"`
}

// DefaultRetryPolicy is a sensible default (3 attempts, exponential backoff capped at 60s).
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts:    3,
	BackoffBaseSec: 5,
	BackoffMaxSec:  60,
}

// RetryRecord is an immutable, hashed record of a retry decision.
// DecidedAt is excluded from the hash (content-identity).
type RetryRecord struct {
	RetryID     string `json:"retry_id"`
	TaskID      string `json:"task_id"`
	ExecutionID string `json:"execution_id"`
	TenantID    string `json:"tenant_id"`
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"max_attempts"`
	BackoffSec  int    `json:"backoff_sec"`
	Reason      string `json:"reason"`
	DecidedAt   string `json:"decided_at"` // excluded from hash
	Hash        string `json:"hash"`
}

// ShouldRetry returns true when the policy permits another attempt.
func ShouldRetry(policy RetryPolicy, attempt int) bool {
	return attempt < policy.MaxAttempts
}

// ComputeRetryBackoff returns the deterministic backoff seconds for the given attempt.
// Uses clamped linear backoff: base * attempt, capped at max.
func ComputeRetryBackoff(policy RetryPolicy, attempt int) int {
	if policy.BackoffBaseSec <= 0 {
		return 0
	}
	v := policy.BackoffBaseSec * attempt
	if v > policy.BackoffMaxSec {
		return policy.BackoffMaxSec
	}
	return v
}

// NewRetryRecord builds a hashed retry record for the next attempt.
func NewRetryRecord(retryID, taskID, executionID, tenantID, reason string, attempt int, policy RetryPolicy) RetryRecord {
	r := RetryRecord{
		RetryID:     retryID,
		TaskID:      taskID,
		ExecutionID: executionID,
		TenantID:    tenantID,
		Attempt:     attempt,
		MaxAttempts: policy.MaxAttempts,
		BackoffSec:  ComputeRetryBackoff(policy, attempt),
		Reason:      reason,
		DecidedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.Hash = computeRetryHash(r)
	return r
}

// VerifyRetryRecord recomputes the hash and returns (true, "") when it matches.
func VerifyRetryRecord(r RetryRecord) (bool, string) {
	stored := r.Hash
	r.Hash = ""
	expected := computeRetryHash(r)
	if stored != expected {
		return false, fmt.Sprintf("retry %q hash mismatch: stored=%s computed=%s", r.RetryID, stored, expected)
	}
	return true, ""
}

func computeRetryHash(r RetryRecord) string {
	h := sha256.New()
	fmt.Fprintf(h, "retry:%s|task:%s|exec:%s|tenant:%s|attempt:%d|max:%d|backoff:%d|reason:%s",
		r.RetryID, r.TaskID, r.ExecutionID, r.TenantID,
		r.Attempt, r.MaxAttempts, r.BackoffSec, r.Reason)
	return fmt.Sprintf("%x", h.Sum(nil))
}
