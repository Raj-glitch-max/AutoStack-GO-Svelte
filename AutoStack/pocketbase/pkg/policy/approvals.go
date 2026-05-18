package policy

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// ApprovalStatus tracks the lifecycle of an approval request.
type ApprovalStatus string

const (
	ApprovalStatusPending   ApprovalStatus = "pending"
	ApprovalStatusGranted   ApprovalStatus = "granted"
	ApprovalStatusDenied    ApprovalStatus = "denied"
	ApprovalStatusExpired   ApprovalStatus = "expired"
	ApprovalStatusEscalated ApprovalStatus = "escalated"
)

// ApprovalRequest is an immutable record of a pending approval.
// Once created, the request itself is never mutated — decisions are separate records.
type ApprovalRequest struct {
	RequestID   string         `json:"request_id"`
	ExecutionID string         `json:"execution_id"`
	TenantID    string         `json:"tenant_id"`
	Action      string         `json:"action"`
	RequestedBy string         `json:"requested_by"`
	ExpiresAt   string         `json:"expires_at"`
	RequestedAt string         `json:"requested_at"`
	Hash        string         `json:"hash"`
}

// ApprovalDecision is an immutable record of an operator decision on an approval request.
// Granted and denied decisions are BOTH persisted — denials are never silently dropped.
type ApprovalDecision struct {
	DecisionID string         `json:"decision_id"`
	RequestID  string         `json:"request_id"`
	OperatorID string         `json:"operator_id"`
	Status     ApprovalStatus `json:"status"` // granted or denied
	Reason     string         `json:"reason"`
	DecidedAt  string         `json:"decided_at"`
	Hash       string         `json:"hash"`
}

// NewApprovalRequest creates an immutable ApprovalRequest with its hash computed.
func NewApprovalRequest(requestID, executionID, tenantID, action, requestedBy, expiresAt string) ApprovalRequest {
	r := ApprovalRequest{
		RequestID:   requestID,
		ExecutionID: executionID,
		TenantID:    tenantID,
		Action:      action,
		RequestedBy: requestedBy,
		ExpiresAt:   expiresAt,
		RequestedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.Hash = computeRequestHash(r)
	return r
}

// NewApprovalDecision creates an immutable ApprovalDecision with its hash computed.
func NewApprovalDecision(decisionID, requestID, operatorID string, status ApprovalStatus, reason string) ApprovalDecision {
	d := ApprovalDecision{
		DecisionID: decisionID,
		RequestID:  requestID,
		OperatorID: operatorID,
		Status:     status,
		Reason:     reason,
		DecidedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	d.Hash = computeApprovalDecisionHash(d)
	return d
}

// IsApprovalExpired returns true when the request's ExpiresAt has passed.
func IsApprovalExpired(req ApprovalRequest, now string) bool {
	return req.ExpiresAt != "" && req.ExpiresAt <= now
}

// ApprovalQueue is an append-only store of approval requests and decisions.
// Thread-safe. Requests are never deleted or mutated.
type ApprovalQueue struct {
	mu        sync.RWMutex
	requests  []ApprovalRequest
	decisions map[string]ApprovalDecision // requestID → decision (first decision wins)
	reqIndex  map[string]int              // requestID → requests index
}

// NewApprovalQueue creates an empty ApprovalQueue.
func NewApprovalQueue() *ApprovalQueue {
	return &ApprovalQueue{
		decisions: make(map[string]ApprovalDecision),
		reqIndex:  make(map[string]int),
	}
}

// Request appends an approval request. Errors on: invalid hash, duplicate requestID.
func (q *ApprovalQueue) Request(req ApprovalRequest) error {
	if ok, reason := VerifyApprovalRequestHash(req); !ok {
		return fmt.Errorf("approval request hash invalid: %s", reason)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, dup := q.reqIndex[req.RequestID]; dup {
		return fmt.Errorf("duplicate approval request ID %q", req.RequestID)
	}
	q.reqIndex[req.RequestID] = len(q.requests)
	q.requests = append(q.requests, req)
	return nil
}

// Decide records an approval decision. Errors on: invalid hash, unknown request, or already decided.
func (q *ApprovalQueue) Decide(decision ApprovalDecision) error {
	if ok, reason := VerifyApprovalDecisionHash(decision); !ok {
		return fmt.Errorf("approval decision hash invalid: %s", reason)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.reqIndex[decision.RequestID]; !ok {
		return fmt.Errorf("approval request %q not found", decision.RequestID)
	}
	if _, exists := q.decisions[decision.RequestID]; exists {
		return fmt.Errorf("approval request %q already has a decision", decision.RequestID)
	}
	q.decisions[decision.RequestID] = decision
	return nil
}

// FindRequest returns the request with the given ID, or (zero, false) if absent.
func (q *ApprovalQueue) FindRequest(requestID string) (ApprovalRequest, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	idx, ok := q.reqIndex[requestID]
	if !ok {
		return ApprovalRequest{}, false
	}
	return q.requests[idx], true
}

// DecisionFor returns the decision for the given requestID, or (zero, false) if none yet.
func (q *ApprovalQueue) DecisionFor(requestID string) (ApprovalDecision, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	d, ok := q.decisions[requestID]
	return d, ok
}

// PendingRequests returns all requests that have no decision and have not expired.
func (q *ApprovalQueue) PendingRequests(now string) []ApprovalRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()
	var out []ApprovalRequest
	for _, r := range q.requests {
		if _, decided := q.decisions[r.RequestID]; decided {
			continue
		}
		if IsApprovalExpired(r, now) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// AllRequests returns a copy of all requests in insertion order.
func (q *ApprovalQueue) AllRequests() []ApprovalRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]ApprovalRequest, len(q.requests))
	copy(out, q.requests)
	return out
}

// VerifyApprovalRequestHash recomputes the hash.
func VerifyApprovalRequestHash(r ApprovalRequest) (bool, string) {
	stored := r.Hash
	r.Hash = ""
	expected := computeRequestHash(r)
	if stored != expected {
		return false, fmt.Sprintf("request %q hash mismatch: stored=%s computed=%s",
			r.RequestID, stored, expected)
	}
	return true, ""
}

// VerifyApprovalDecisionHash recomputes the hash.
func VerifyApprovalDecisionHash(d ApprovalDecision) (bool, string) {
	stored := d.Hash
	d.Hash = ""
	expected := computeApprovalDecisionHash(d)
	if stored != expected {
		return false, fmt.Sprintf("approval decision %q hash mismatch: stored=%s computed=%s",
			d.DecisionID, stored, expected)
	}
	return true, ""
}

func computeRequestHash(r ApprovalRequest) string {
	h := sha256.New()
	fmt.Fprintf(h, "req:%s|exec:%s|tenant:%s|action:%s|by:%s|expires:%s|ts:%s",
		r.RequestID, r.ExecutionID, r.TenantID, r.Action, r.RequestedBy, r.ExpiresAt, r.RequestedAt)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func computeApprovalDecisionHash(d ApprovalDecision) string {
	h := sha256.New()
	fmt.Fprintf(h, "adecision:%s|req:%s|operator:%s|status:%s|reason:%s|ts:%s",
		d.DecisionID, d.RequestID, d.OperatorID, d.Status, d.Reason, d.DecidedAt)
	return fmt.Sprintf("%x", h.Sum(nil))
}
