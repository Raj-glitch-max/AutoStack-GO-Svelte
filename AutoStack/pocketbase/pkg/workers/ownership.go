package workers

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// OwnershipRecord is an immutable, hashed record of a task ownership transfer.
// TransferredAt is excluded from the hash (content-identity).
type OwnershipRecord struct {
	RecordID      string `json:"record_id"`
	TaskID        string `json:"task_id"`
	ExecutionID   string `json:"execution_id"`
	TenantID      string `json:"tenant_id"`
	FromWorker    string `json:"from_worker"` // empty on initial assignment
	ToWorker      string `json:"to_worker"`
	Reason        string `json:"reason"`
	TransferredAt string `json:"transferred_at"` // excluded from hash
	RecordHash    string `json:"record_hash"`
}

// NewOwnershipRecord creates a hashed ownership record.
func NewOwnershipRecord(recordID, taskID, executionID, tenantID, fromWorker, toWorker, reason string) OwnershipRecord {
	r := OwnershipRecord{
		RecordID:      recordID,
		TaskID:        taskID,
		ExecutionID:   executionID,
		TenantID:      tenantID,
		FromWorker:    fromWorker,
		ToWorker:      toWorker,
		Reason:        reason,
		TransferredAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.RecordHash = computeOwnershipHash(r)
	return r
}

// VerifyOwnershipRecord recomputes the hash and returns (true, "") when it matches.
func VerifyOwnershipRecord(r OwnershipRecord) (bool, string) {
	stored := r.RecordHash
	r.RecordHash = ""
	expected := computeOwnershipHash(r)
	if stored != expected {
		return false, fmt.Sprintf("ownership record %q hash mismatch: stored=%s computed=%s",
			r.RecordID, stored, expected)
	}
	return true, ""
}

// OwnershipHistory is an append-only log of all task ownership records.
type OwnershipHistory struct {
	mu      sync.RWMutex
	records []OwnershipRecord
}

// NewOwnershipHistory returns an empty ownership history.
func NewOwnershipHistory() *OwnershipHistory {
	return &OwnershipHistory{}
}

// Append adds a record. Returns an error if the hash is invalid.
func (h *OwnershipHistory) Append(r OwnershipRecord) error {
	if ok, reason := VerifyOwnershipRecord(r); !ok {
		return fmt.Errorf("ownership: invalid record hash: %s", reason)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

// ForTask returns all ownership records for a given taskID in append order.
func (h *OwnershipHistory) ForTask(taskID string) []OwnershipRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []OwnershipRecord
	for _, r := range h.records {
		if r.TaskID == taskID {
			out = append(out, r)
		}
	}
	return out
}

// CurrentOwner returns the most recent ToWorker for a given taskID, or "" if none.
func (h *OwnershipHistory) CurrentOwner(taskID string) string {
	recs := h.ForTask(taskID)
	if len(recs) == 0 {
		return ""
	}
	return recs[len(recs)-1].ToWorker
}

func computeOwnershipHash(r OwnershipRecord) string {
	h := sha256.New()
	fmt.Fprintf(h, "ownership:%s|task:%s|exec:%s|tenant:%s|from:%s|to:%s|reason:%s",
		r.RecordID, r.TaskID, r.ExecutionID, r.TenantID,
		r.FromWorker, r.ToWorker, r.Reason)
	return fmt.Sprintf("%x", h.Sum(nil))
}
